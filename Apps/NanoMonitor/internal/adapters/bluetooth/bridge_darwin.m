//go:build darwin && cgo

#import <Foundation/Foundation.h>
#import <CoreBluetooth/CoreBluetooth.h>
#include "bridge.h"
#include <string.h>

@interface NanoBLE : NSObject <CBCentralManagerDelegate, CBPeripheralDelegate>
@property(nonatomic, strong) dispatch_queue_t queue;
@property(nonatomic, strong) CBCentralManager *manager;
@property(nonatomic, strong) NSMutableDictionary<NSString *, CBPeripheral *> *devices;
@property(nonatomic, strong) NSMutableDictionary<NSString *, NSString *> *names;
@property(nonatomic, strong) CBPeripheral *peripheral;
@property(nonatomic, strong) CBCharacteristic *notify;
@property(nonatomic, strong) CBCharacteristic *writer;
@property(nonatomic, strong) NSMutableArray<NSDictionary *> *notifications;
@property(nonatomic) NSInteger status;
@property(nonatomic) BOOL notifySettled;
@property(nonatomic) BOOL writerSettled;
@property(nonatomic) BOOL armed;
@end

@implementation NanoBLE
- (instancetype)init {
    self = [super init];
    if (self) {
        _queue = dispatch_queue_create("nanomonitor.bluetooth", DISPATCH_QUEUE_SERIAL);
        _devices = [NSMutableDictionary dictionary];
        _names = [NSMutableDictionary dictionary];
        _notifications = [NSMutableArray array];
        _manager = [[CBCentralManager alloc] initWithDelegate:self queue:_queue options:@{CBCentralManagerOptionShowPowerAlertKey: @YES}];
    }
    return self;
}

- (void)centralManagerDidUpdateState:(CBCentralManager *)central {
    if (central.state != CBManagerStatePoweredOn && self.peripheral) self.status = -2;
}

- (void)centralManager:(CBCentralManager *)central didDiscoverPeripheral:(CBPeripheral *)peripheral advertisementData:(NSDictionary<NSString *, id> *)advertisementData RSSI:(NSNumber *)RSSI {
    NSString *name = advertisementData[CBAdvertisementDataLocalNameKey] ?: peripheral.name ?: @"";
    NSString *compact = [[name lowercaseString] stringByReplacingOccurrencesOfString:@" " withString:@""];
    if (![compact hasPrefix:@"osmonano"]) return;
    NSData *manufacturer = advertisementData[CBAdvertisementDataManufacturerDataKey];
    if (manufacturer.length >= 4) {
        const uint8_t *b = manufacturer.bytes;
        unsigned company = b[0] | (b[1] << 8);
        if (company != 0x08aa && company != 0xf7aa && company != 0xe5c0) return;
        unsigned model = b[2] | (b[3] << 8);
        if (manufacturer.length >= 14 && (b[7] & 4)) {
            if ((b[12] | (b[13] << 8)) != 222) return;
        } else if (model != 0 && model != 0x19) {
            return;
        }
    }
    if (self.devices.count >= 64 && !self.devices[peripheral.identifier.UUIDString]) return;
    self.devices[peripheral.identifier.UUIDString] = peripheral;
    self.names[peripheral.identifier.UUIDString] = name;
}

- (void)centralManager:(CBCentralManager *)central didConnectPeripheral:(CBPeripheral *)peripheral {
    if (peripheral != self.peripheral) return;
    peripheral.delegate = self;
    [peripheral discoverServices:@[[CBUUID UUIDWithString:@"FFF0"]]];
}

- (void)centralManager:(CBCentralManager *)central didFailToConnectPeripheral:(CBPeripheral *)peripheral error:(NSError *)error {
    if (peripheral == self.peripheral) self.status = -3;
}

- (void)centralManager:(CBCentralManager *)central didDisconnectPeripheral:(CBPeripheral *)peripheral error:(NSError *)error {
    if (peripheral == self.peripheral) self.status = -4;
}

- (void)peripheral:(CBPeripheral *)peripheral didDiscoverServices:(NSError *)error {
    if (peripheral != self.peripheral) return;
    if (error) { self.status = -5; return; }
    for (CBService *service in peripheral.services) {
        if ([service.UUID isEqual:[CBUUID UUIDWithString:@"FFF0"]]) {
            [peripheral discoverCharacteristics:@[[CBUUID UUIDWithString:@"FFF4"], [CBUUID UUIDWithString:@"FFF5"]] forService:service];
            return;
        }
    }
    self.status = -5;
}

- (void)armIfReady {
    if (self.status < 0) return;
    if (!self.armed && self.notifySettled && self.writerSettled && self.notify.isNotifying) {
        self.armed = YES;
        const uint8_t bytes[] = {1, 0};
        [self.peripheral writeValue:[NSData dataWithBytes:bytes length:2] forCharacteristic:self.notify type:CBCharacteristicWriteWithResponse];
    }
}

- (void)peripheral:(CBPeripheral *)peripheral didDiscoverCharacteristicsForService:(CBService *)service error:(NSError *)error {
    if (peripheral != self.peripheral) return;
    if (error) { self.status = -5; return; }
    for (CBCharacteristic *characteristic in service.characteristics) {
        if ([characteristic.UUID isEqual:[CBUUID UUIDWithString:@"FFF4"]]) self.notify = characteristic;
        if ([characteristic.UUID isEqual:[CBUUID UUIDWithString:@"FFF5"]]) self.writer = characteristic;
    }
    if (!self.notify || !self.writer || !(self.writer.properties & CBCharacteristicPropertyWriteWithoutResponse)) {
        self.status = -5;
        return;
    }
    if (!(self.notify.properties & (CBCharacteristicPropertyNotify | CBCharacteristicPropertyIndicate))) {
        self.status = -5;
        return;
    }
    [peripheral setNotifyValue:YES forCharacteristic:self.notify];
    if (self.writer.properties & (CBCharacteristicPropertyNotify | CBCharacteristicPropertyIndicate)) {
        [peripheral setNotifyValue:YES forCharacteristic:self.writer];
    } else {
        self.writerSettled = YES;
    }
}

- (void)peripheral:(CBPeripheral *)peripheral didUpdateNotificationStateForCharacteristic:(CBCharacteristic *)characteristic error:(NSError *)error {
    if (peripheral != self.peripheral) return;
    if (characteristic == self.notify) {
        self.notifySettled = YES;
        if (error || !characteristic.isNotifying) { self.status = -5; return; }
    }
    if (characteristic == self.writer) self.writerSettled = YES;
    [self armIfReady];
}

- (void)peripheral:(CBPeripheral *)peripheral didWriteValueForCharacteristic:(CBCharacteristic *)characteristic error:(NSError *)error {
    if (peripheral != self.peripheral || characteristic != self.notify) return;
    if (self.status < 0) return;
    self.status = error ? -5 : 1;
}

- (void)peripheral:(CBPeripheral *)peripheral didUpdateValueForCharacteristic:(CBCharacteristic *)characteristic error:(NSError *)error {
    if (peripheral != self.peripheral || error || !characteristic.value.length) return;
    if (characteristic != self.notify && characteristic != self.writer) return;
    if (characteristic.value.length > 4096 || self.notifications.count >= 128) {
        self.status = -6;
        return;
    }
    [self.notifications addObject:@{@"channel": characteristic == self.notify ? @4 : @5, @"data": [characteristic.value copy]}];
}
@end

void *nano_ble_create(void) {
    @autoreleasepool {
        if (![[NSBundle mainBundle] objectForInfoDictionaryKey:@"NSBluetoothAlwaysUsageDescription"]) return NULL;
        return (__bridge_retained void *)[[NanoBLE alloc] init];
    }
}

int nano_ble_power(void *handle) {
    NanoBLE *client = (__bridge NanoBLE *)handle;
    __block int result;
    dispatch_sync(client.queue, ^{ result = (int)client.manager.state; });
    return result;
}

int nano_ble_scan(void *handle) {
    @autoreleasepool {
        NanoBLE *client = (__bridge NanoBLE *)handle;
        __block int result = 0;
        dispatch_sync(client.queue, ^{
            if (client.manager.state != CBManagerStatePoweredOn) { result = -2; return; }
            [client.devices removeAllObjects];
            [client.names removeAllObjects];
            for (CBPeripheral *peripheral in [client.manager retrieveConnectedPeripheralsWithServices:@[[CBUUID UUIDWithString:@"FFF0"]]]) {
                NSString *name = peripheral.name ?: @"";
                if ([[[name lowercaseString] stringByReplacingOccurrencesOfString:@" " withString:@""] hasPrefix:@"osmonano"]) {
                    client.devices[peripheral.identifier.UUIDString] = peripheral;
                    client.names[peripheral.identifier.UUIDString] = name;
                }
            }
            [client.manager scanForPeripheralsWithServices:nil options:nil];
        });
        return result;
    }
}

int nano_ble_devices(void *handle, char *buffer, size_t capacity) {
    @autoreleasepool {
        NanoBLE *client = (__bridge NanoBLE *)handle;
        __block int result = 0;
        dispatch_sync(client.queue, ^{
            [client.manager stopScan];
            NSMutableArray *items = [NSMutableArray array];
            for (NSString *identifier in client.names) [items addObject:@{@"id": identifier, @"name": client.names[identifier]}];
            NSData *json = [NSJSONSerialization dataWithJSONObject:items options:0 error:nil];
            if (!json || json.length > capacity) { result = -6; return; }
            memcpy(buffer, json.bytes, json.length);
            result = (int)json.length;
        });
        return result;
    }
}

int nano_ble_connect(void *handle, const char *identifier) {
    @autoreleasepool {
        NanoBLE *client = (__bridge NanoBLE *)handle;
        NSString *key = [NSString stringWithUTF8String:identifier];
        __block int result = 0;
        dispatch_sync(client.queue, ^{
            CBPeripheral *peripheral = client.devices[key];
            if (!peripheral) { result = -3; return; }
            if (client.peripheral && client.peripheral != peripheral) [client.manager cancelPeripheralConnection:client.peripheral];
            [client.manager stopScan];
            client.peripheral = peripheral;
            client.notify = nil;
            client.writer = nil;
            client.notifySettled = NO;
            client.writerSettled = NO;
            client.armed = NO;
            client.status = 0;
            [client.notifications removeAllObjects];
            peripheral.delegate = client;
            [client.manager connectPeripheral:peripheral options:nil];
        });
        return result;
    }
}

int nano_ble_status(void *handle) {
    NanoBLE *client = (__bridge NanoBLE *)handle;
    __block int result;
    dispatch_sync(client.queue, ^{ result = (int)client.status; });
    return result;
}

int nano_ble_write(void *handle, const uint8_t *bytes, size_t length) {
    @autoreleasepool {
        NanoBLE *client = (__bridge NanoBLE *)handle;
        __block int result = 0;
        dispatch_sync(client.queue, ^{
            if (client.status != 1) { result = -4; return; }
            if (length > [client.peripheral maximumWriteValueLengthForType:CBCharacteristicWriteWithoutResponse]) { result = -7; return; }
            if (!client.peripheral.canSendWriteWithoutResponse) { result = 1; return; }
            [client.peripheral writeValue:[NSData dataWithBytes:bytes length:length] forCharacteristic:client.writer type:CBCharacteristicWriteWithoutResponse];
        });
        return result;
    }
}

int nano_ble_read(void *handle, uint8_t *buffer, size_t capacity, int *channel) {
    @autoreleasepool {
        NanoBLE *client = (__bridge NanoBLE *)handle;
        __block int result = 0;
        dispatch_sync(client.queue, ^{
            if (client.status < 0) { result = (int)client.status; return; }
            if (!client.notifications.count) return;
            NSDictionary *item = client.notifications[0];
            NSData *data = item[@"data"];
            if (data.length > capacity) { result = -6; return; }
            memcpy(buffer, data.bytes, data.length);
            *channel = [item[@"channel"] intValue];
            result = (int)data.length;
            [client.notifications removeObjectAtIndex:0];
        });
        return result;
    }
}

void nano_ble_close(void *handle) {
    @autoreleasepool {
        NanoBLE *client = (__bridge_transfer NanoBLE *)handle;
        dispatch_sync(client.queue, ^{
            [client.manager stopScan];
            client.peripheral.delegate = nil;
            if (client.peripheral) [client.manager cancelPeripheralConnection:client.peripheral];
            client.manager.delegate = nil;
            [client.notifications removeAllObjects];
            client.peripheral = nil;
        });
    }
}
