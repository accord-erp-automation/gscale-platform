package main

import "testing"

func TestIsRetryableGoDEXPrintError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  string
		want bool
	}{
		{name: "download no device", err: "download qr graphic: usb write: libusb: no device [code -4]", want: true},
		{name: "open busy", err: "open GoDEX G500 usb: resource busy", want: true},
		{name: "print command no retry", err: "send print command 3: usb write: libusb: no device [code -4]", want: false},
		{name: "validation no retry", err: "company, product, kg, and epc are required", want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isRetryableGoDEXPrintError(tt.err); got != tt.want {
				t.Fatalf("isRetryableGoDEXPrintError() = %v, want %v", got, tt.want)
			}
		})
	}
}
