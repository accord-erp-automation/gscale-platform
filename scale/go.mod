module scale

go 1.25.0

require (
	bridge v0.0.0
	core v0.0.0
	batch v0.0.0
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07
	godex v0.0.0
)

require (
	github.com/google/gousb v1.1.3 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	golang.org/x/image v0.39.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

replace bridge => ../bridge

replace core => ../core

replace batch => ../batch

replace godex => ../godex
