package secret

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
)

func NetworkName(ctx context.Context) (string, error) {
	value, err := prompt(ctx, "Nano를 연결할 WPA2 공유기의 Wi-Fi 이름을 입력해 주세요.", false)
	return string(value), err
}

func Password(ctx context.Context, ssid string) ([]byte, error) {
	return prompt(ctx, "Wi-Fi: "+ssid+"\nNano를 연결할 WPA2 공유기의 비밀번호를 입력해 주세요. 비밀번호는 저장하지 않습니다.", true)
}

func prompt(ctx context.Context, message string, hidden bool) ([]byte, error) {
	if runtime.GOOS != "darwin" {
		return nil, errors.New("보안 입력창은 현재 macOS에서 지원합니다")
	}
	result, err := exec.CommandContext(ctx, "osascript", "-e", promptScript(hidden), "--", message).Output()
	if err != nil {
		clear(result)
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("입력이 취소됐거나 보안 입력창을 열지 못했습니다")
	}
	return bytes.TrimSuffix(result, []byte{'\n'}), nil
}

func promptScript(hidden bool) string {
	script := `on run argv
set answer to display dialog (item 1 of argv) default answer "" with title "Nano Monitor 초기 설정" buttons {"취소", "확인"} default button "확인" cancel button "취소" giving up after 180`
	if hidden {
		script += " with hidden answer"
	}
	script += `
if gave up of answer then error number -128
return text returned of answer
end run`
	return script
}
