package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// stdin is read through one reader, so a second answer piped in is the
// second line and not lost to the first one's buffer.
var stdin = bufio.NewReader(os.Stdin)

// secret reads what (a password, a key) unseen at a terminal, asked again
// to confirm when confirm is set; piped in, it is the next line of stdin.
func secret(what, prompt string, confirm bool) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		line, err := stdin.ReadString('\n')
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if err != nil {
				return "", fmt.Errorf("no %s on stdin: %w", what, err)
			}
			return "", fmt.Errorf("the %s is empty", what)
		}
		return line, nil
	}
	read := func(p string) (string, error) {
		fmt.Fprint(os.Stderr, p)
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	pass, err := read(prompt)
	if err != nil {
		return "", err
	}
	if pass == "" {
		return "", fmt.Errorf("the %s is empty", what)
	}
	if confirm {
		again, err := read("Again: ")
		if err != nil {
			return "", err
		}
		if again != pass {
			return "", fmt.Errorf("the %ss differ", what)
		}
	}
	return pass, nil
}
