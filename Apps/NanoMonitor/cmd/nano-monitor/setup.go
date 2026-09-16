package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"strings"
	"time"

	"nanomonitor/internal/adapters/bluetooth"
	"nanomonitor/internal/adapters/ffplay"
	"nanomonitor/internal/adapters/nano"
	"nanomonitor/internal/adapters/network"
	"nanomonitor/internal/adapters/secret"
	"nanomonitor/internal/application"
	"nanomonitor/internal/domain"
)

func setup(ctx context.Context, args []string, out, log io.Writer) error {
	flags := flag.NewFlagSet("nano-monitor setup", flag.ContinueOnError)
	flags.SetOutput(log)
	ssid := flags.String("ssid", "", "WPA2 공유기 이름. 생략하면 입력창을 엽니다")
	iface := flags.String("interface", "", "공유기에 연결된 네트워크 장치")
	deviceID := flags.String("device", "", "검색한 Nano의 정확한 이름 또는 Bluetooth ID")
	cameraIP := flags.String("camera", "", "공유기에서 확인한 Nano 주소. 생략하면 가입 후 자동 검색합니다")
	list := flags.Bool("list", false, "Bluetooth에서 Nano를 검색하고 종료합니다")
	restore := flags.Bool("restore-ap", false, "Nano 자체 Wi-Fi로 복원하도록 요청합니다")
	monitor := flags.Bool("monitor", false, "등록 및 네트워크 확인 후 영상 창을 엽니다")
	playerPath := flags.String("ffplay", "ffplay", "FFplay 실행 파일 경로")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || (*list && (*restore || *monitor || *ssid != "" || *cameraIP != "")) || (*restore && (*monitor || *ssid != "" || *cameraIP != "")) {
		return errors.New("setup의 인수 조합이 올바르지 않습니다. -h로 사용법을 확인해 주세요")
	}
	var lan network.LAN
	var err error
	var target netip.Addr
	if *cameraIP != "" {
		target, err = network.ParseCamera(*cameraIP)
		if err != nil {
			return err
		}
	}
	if !*list && !*restore {
		lan, err = network.Select(*iface, target)
		if err != nil {
			return err
		}
		if !target.IsValid() {
			if _, err := network.Hosts(lan); err != nil {
				return err
			}
		}
	}
	var player *ffplay.Player
	if *monitor {
		player, err = ffplay.New(*playerPath)
		if err != nil {
			return err
		}
	}
	client, err := bluetooth.New()
	if err != nil {
		return err
	}
	defer client.Close()
	fmt.Fprintln(log, "Bluetooth에서 Nano를 검색합니다. 접근 요청이 나오면 허용해 주세요.")
	devices, err := client.Discover(ctx)
	if err != nil {
		return err
	}
	if *list {
		for _, device := range devices {
			fmt.Fprintf(out, "%s  %q\n", device.ID, device.Name)
		}
		if len(devices) == 0 {
			fmt.Fprintln(log, "Nano가 검색되지 않았습니다. 전원과 Bluetooth 접근 권한을 확인해 주세요.")
		}
		return nil
	}
	device, err := selectDevice(devices, *deviceID)
	if err != nil {
		return err
	}
	report := func(message string) { fmt.Fprintln(log, message) }
	provisioner := nano.Provisioner{Link: client, Report: report}
	if *restore {
		restoreCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		if err := provisioner.Restore(restoreCtx, device); err != nil {
			return err
		}
		fmt.Fprintln(out, "카메라가 자체 Wi-Fi 복원 요청을 받아들였습니다. Nano의 화면과 Wi-Fi 목록에서 복원을 확인해 주세요.")
		return nil
	}
	if *ssid == "" {
		*ssid, err = secret.NetworkName(ctx)
		if err != nil {
			return err
		}
	}
	passphrase, err := secret.Password(ctx, *ssid)
	if err != nil {
		return err
	}
	defer clear(passphrase)
	setupCtx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	locator := nano.Locator{LocalAddress: lan.Address, Report: report, Candidates: func(ctx context.Context) ([]netip.Addr, error) {
		if target.IsValid() {
			return []netip.Addr{target}, nil
		}
		return network.Discover(ctx, lan)
	}}
	result, err := application.Setup(setupCtx, device, domain.Network{SSID: *ssid, Passphrase: passphrase}, provisioner, locator)
	clear(passphrase)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Nano의 공유기 연결을 확인했습니다. 주소: %s\n", result.Address)
	if *monitor {
		camera := nano.Camera{Address: result.Address, LocalAddress: lan.Address, ExpectedName: result.Identity.Name, Report: report}
		return application.Monitor(ctx, camera, player)
	}
	fmt.Fprintf(out, "실행: nano-monitor -interface %s -camera %s -name %s\n", shellQuote(lan.Interface), result.Address, shellQuote(result.Identity.Name))
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func selectDevice(devices []domain.Device, selection string) (domain.Device, error) {
	var matches []domain.Device
	for _, device := range devices {
		if selection == "" || device.ID == selection || device.Name == selection {
			matches = append(matches, device)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) == 0 {
		return domain.Device{}, errors.New("선택할 Nano가 없습니다. setup -list로 검색 결과를 확인해 주세요")
	}
	return domain.Device{}, errors.New("Nano가 여러 대입니다. setup -list로 확인한 Bluetooth ID를 -device에 지정해 주세요")
}
