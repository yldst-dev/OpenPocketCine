#include <stddef.h>
#include <stdint.h>

void *nano_ble_create(void);
int nano_ble_power(void *handle);
int nano_ble_scan(void *handle);
int nano_ble_devices(void *handle, char *buffer, size_t capacity);
int nano_ble_connect(void *handle, const char *identifier);
int nano_ble_status(void *handle);
int nano_ble_write(void *handle, const uint8_t *bytes, size_t length);
int nano_ble_read(void *handle, uint8_t *buffer, size_t capacity, int *channel);
void nano_ble_close(void *handle);
