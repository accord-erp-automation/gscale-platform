package main

import (
	"errors"
	"strings"
	"time"
)

func sendRawRetry(device string, payload []byte, retries int, delay time.Duration) error {
	if retries < 1 {
		retries = 1
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		err := zebraSendRaw(device, payload)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isBusyLikeError(err) {
			return err
		}
		time.Sleep(delay)
	}
	if lastErr == nil {
		lastErr = errors.New("zebra: send retry failed")
	}
	return lastErr
}

func sendSGDRetry(device, command string, retries int, delay time.Duration) error {
	if retries < 1 {
		retries = 1
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		err := zebraSendSGD(device, command)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isBusyLikeError(err) {
			return err
		}
		time.Sleep(delay)
	}
	if lastErr == nil {
		lastErr = errors.New("zebra: sgd retry failed")
	}
	return lastErr
}

func queryVarRetry(device, key string, timeout time.Duration, retries int, delay time.Duration) string {
	if retries < 1 {
		retries = 1
	}
	for i := 0; i < retries; i++ {
		v, err := queryVarSoft(device, key, timeout)
		if err == nil {
			v = strings.TrimSpace(strings.Trim(v, "\""))
			if v != "" && v != "?" {
				return v
			}
		}
		time.Sleep(delay)
	}
	return ""
}

func queryVarSoft(device, key string, timeout time.Duration) (string, error) {
	return queryZebraSGDVar(device, key, timeout)
}

func isBusyLikeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "resource busy") || strings.Contains(msg, "device or resource busy") || strings.Contains(msg, "temporarily unavailable")
}
