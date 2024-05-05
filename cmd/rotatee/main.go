package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	version = "0.1.0"
)

type State struct {
	prefix  string
	dir     string
	format  string
	maxsize int
	cur     int
	ignore  bool

	sigch chan os.Signal
}

func usage() {
	fmt.Fprintf(os.Stderr, `%s v%s
Usage: %s [<option>] [<fileprefix>]

tee(1) with file rotation.

Examples:

  # writes output to files in the current directory prefixed with "stdout"
  rotatee

  # writes output to files in /tmp prefixed with "output"
  rotatee --dir=/tmp output

Options:

`, path.Base(os.Args[0]), version, os.Args[0])
	flag.PrintDefaults()
}

func main() {
	dir := flag.String("dir", ".", "output directory")
	maxsize := flag.Int("maxsize", 100, "max file size (MiB)")
	format := flag.String("format", time.RFC3339+".log", "timestamp")
	ignore := flag.Bool("ignore", false, "ignore SIGTERM")

	flag.Usage = func() { usage() }
	flag.Parse()

	prefix := "stdout"

	if flag.NArg() > 0 {
		prefix = flag.Arg(0)
	}

	state := &State{
		prefix:  prefix,
		format:  *format,
		dir:     *dir,
		maxsize: *maxsize * 1024 * 1024,
		ignore:  *ignore,
		sigch:   make(chan os.Signal, 1),
	}

	if err := os.MkdirAll(*dir, 0700); err != nil {
		log.Fatalln(err)
	}

	if err := state.initialize(); err != nil {
		log.Fatalln(err)
	}

	go state.signal()

	if err := state.run(); err != nil {
		log.Fatalln(err)
	}
}

func (state *State) initialize() error {
	glob := filepath.Join(state.dir, "*.rotatee")
	matches, err := filepath.Glob(glob)
	if err != nil {
		return fmt.Errorf("%s:%w", glob, err)
	}

	for _, v := range matches {
		if err := state.rename(v); err != nil {
			log.Println(v, err)
		}
	}

	return nil
}

func (state *State) signal() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP, syscall.SIGTERM)

	for {
		sig := <-ch
		switch sig {
		case syscall.SIGHUP:
		case syscall.SIGTERM:
			if state.ignore {
				continue
			}
		default:
			log.Printf("unhandled signal received: %s\n", sig)
			continue
		}
		state.sigch <- sig
	}
}

func (state *State) path(t time.Time) string {
	ts := t.Format(state.format)
	return filepath.Join(state.dir, state.prefix+"."+ts+".rotatee")
}

func (state *State) rename(oldpath string) error {
	newpath := strings.TrimSuffix(oldpath, ".rotatee")
	return os.Rename(oldpath, newpath)
}

func (state *State) run() error {
	path := state.path(time.Now())
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	defer func() {
		_ = state.rename(path)
	}()

	rotate := false

	stdin := bufio.NewScanner(os.Stdin)
	for stdin.Scan() {
		s := stdin.Text()
		fmt.Println(s)

		// Include newline in count.
		if len(s)+1+state.cur > state.maxsize {
			rotate = true
		}

		if rotate {
			rotate = false
			state.cur = 0

			if err := f.Close(); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}

			if err := state.rename(path); err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}

			path = state.path(time.Now())
			f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
		}

		state.cur += len(s) + 1 // newline

		fmt.Fprintln(f, s)

		select {
		case sig := <-state.sigch:
			switch sig {
			case syscall.SIGTERM:
				return nil
			case syscall.SIGHUP:
				rotate = true
			}
		default:
		}
	}

	return stdin.Err()
}
