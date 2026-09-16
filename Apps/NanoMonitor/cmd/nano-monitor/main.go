package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"nanomonitor/internal/adapters/ffplay"
	"nanomonitor/internal/adapters/nano"
	"nanomonitor/internal/adapters/network"
	"nanomonitor/internal/application"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, "오류:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out, log io.Writer) error {
	flags := flag.NewFlagSet("nano-monitor", flag.ContinueOnError)
	flags.SetOutput(log)
	address := flags.String("camera", "", "Nano의 공유기 IPv4 주소. 생략하면 같은 대역을 검색합니다")
	iface := flags.String("interface", "", "사용할 네트워크 장치 이름")
	name := flags.String("name", "", "Nano의 정확한 카메라 Wi-Fi 이름을 확인합니다")
	discover := flags.Bool("discover", false, "포트 7001이 열려 있는 후보 주소만 검색합니다")
	playerPath := flags.String("ffplay", "ffplay", "FFplay 실행 파일 경로")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || (*discover && *address != "") {
		return errors.New("추가 위치 인수는 받지 않습니다. -discover와 -camera는 함께 사용할 수 없습니다")
	}
	var target netip.Addr
	var err error
	if *address != "" {
		target, err = network.ParseCamera(*address)
		if err != nil {
			return err
		}
	}
	var player *ffplay.Player
	if !*discover {
		player, err = ffplay.New(*playerPath)
		if err != nil {
			return err
		}
	}
	lan, err := network.Select(*iface, target)
	if err != nil {
		return err
	}
	if !target.IsValid() {
		fmt.Fprintf(log, "%s의 %s 대역을 검색합니다.\n", lan.Interface, lan.Prefix)
		candidates, err := network.Discover(ctx, lan)
		if err != nil {
			return err
		}
		if *discover {
			if len(candidates) == 0 {
				fmt.Fprintln(log, "해당 대역에서 응답한 카메라 후보가 없습니다.")
				return nil
			}
			for _, candidate := range candidates {
				fmt.Fprintln(out, candidate)
			}
			fmt.Fprintln(log, "위 주소는 서비스 후보입니다. 실제 Nano 여부는 연결할 때 확인합니다.")
			return nil
		}
		if len(candidates) == 0 {
			return errors.New("카메라 후보가 없습니다. Nano를 같은 WPA2 공유기에 연결하고 게스트 네트워크 격리를 확인해 주세요")
		}
		if len(candidates) != 1 {
			for _, candidate := range candidates {
				fmt.Fprintln(out, candidate)
			}
			return errors.New("후보가 여러 개입니다. 확인한 Nano의 주소를 -camera로 지정해 주세요")
		}
		target = candidates[0]
	}
	fmt.Fprintf(log, "%s에서 %s로 연결합니다.\n", lan.Interface, target)
	camera := nano.Camera{
		Address: target, LocalAddress: lan.Address, ExpectedName: *name,
		Report: func(message string) { fmt.Fprintln(log, message) },
	}
	return application.Monitor(ctx, camera, player)
}
