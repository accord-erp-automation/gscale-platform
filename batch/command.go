package batch

import (
	"strings"

	"golang.org/x/text/encoding/charmap"
)

func EncodeCommand(command string) []byte {
	command = strings.TrimRight(command, "\r\n")
	encoded, err := charmap.Windows1251.NewEncoder().String(command)
	if err != nil {
		encoded = command
	}
	return []byte(encoded + "\r\n")
}
