module scale

go 1.25

require (
	bridge v0.0.0
	core v0.0.0
	github.com/tarm/serial v0.0.0-20180830185346-98f6abe2eb07
)

require golang.org/x/sys v0.41.0 // indirect

replace bridge => ../bridge

replace core => ../core
