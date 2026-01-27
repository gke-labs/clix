package main

import (
	"io"
)

type Sandbox interface {
	Run(stdin io.Reader, stdout, stderr io.Writer, script Script, args []string) error
}
