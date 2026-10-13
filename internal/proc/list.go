package proc

// A Process is one running on this computer, as the system lists it: its
// id and its command line, arguments joined by spaces.
type Process struct {
	PID  int
	Args string
}
