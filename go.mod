module appvolume

go 1.24

replace golang.org/x/image => github.com/golang/image v0.24.0

replace golang.org/x/sys => github.com/golang/sys v0.30.0

replace golang.org/x/text => github.com/golang/text v0.22.0

require (
	github.com/gorilla/websocket v1.5.3
	golang.org/x/image v0.24.0
	golang.org/x/sys v0.30.0
)

require golang.org/x/text v0.22.0 // indirect
