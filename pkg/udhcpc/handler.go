package udhcpc

type Info struct {
	IP      string
	Gateway string
	Domain  string
}

type Event struct {
	Type string
	Data Info
}
