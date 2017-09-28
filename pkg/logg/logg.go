package logg

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

var ignoreLocalhost = true
var logLevel = 0

func RecordLocalhostError(r bool) {
	ignoreLocalhost = !r
}

func SetLevel(lv string) {
	switch lv {
	case "dbg":
		logLevel = -1
	case "log":
		logLevel = 0
	case "warn":
		logLevel = 1
	case "err":
		logLevel = 2
	case "off":
		logLevel = 3
	default:
		panic("unexpected log level: " + lv)
	}
}

func timestamp() string {
	t := time.Now()
	mil := t.UnixNano() % 1e9
	mil /= 1e6

	return fmt.Sprintf("%02d%02d %02d:%02d:%02d.%03d", t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), mil)
}

func lead(l string) string {
	return ("[" + l + " " + timestamp() + "] ")
}

// Widnows WSA error messages are way too long to print
// ex: An established connection was aborted by the software in your host machine.write tcp 127.0.0.1:8100->127.0.0.1:52466: wsasend: An established connection was aborted by the software in your host machine.
func tryShortenWSAError(err interface{}) (ret string) {
	defer func() {
		if recover() != nil {
			ret = fmt.Sprintf("%v", err)
		}
	}()

	if e, sysok := err.(*net.OpError).Err.(*os.SyscallError); sysok {
		errno := e.Err.(syscall.Errno)
		if msg, ok := WSAErrno[int(errno)]; ok {
			ret = msg
		} else {
			// messages on linux are short enough
			ret = fmt.Sprintf("C%d, %s", uintptr(errno), e.Error())
		}

		return
	}

	ret = err.(*net.OpError).Err.Error()
	return
}

type msg_t struct {
	dst     string
	message string
}

var msgQueue = make(chan msg_t)

func print(l string, params ...interface{}) {
	l = lead(l)
	m := msg_t{}

	for _, p := range params {
		switch p.(type) {
		case *net.OpError:
			op := p.(*net.OpError)
			if ignoreLocalhost && op.Source != nil && op.Addr != nil {
				if strings.Split(op.Source.String(), ":")[0] == strings.Split(op.Addr.String(), ":")[0] {
					return
				}
			}

			if op.Source == nil && op.Addr == nil {
				l += fmt.Sprintf("%s, %s", op.Op, tryShortenWSAError(p))
			} else if op.Source == nil {
				l += fmt.Sprintf("[%s]-> %v, %s", op.Op, op.Addr, tryShortenWSAError(p))
			} else {
				l += fmt.Sprintf("%v -[%s]-> %v, %s", op.Source, op.Op, op.Addr, tryShortenWSAError(p))
				m.dst, _, _ = net.SplitHostPort(op.Addr.String())
			}
		case *net.DNSError:
			op := p.(*net.DNSError)
			if op.IsTimeout {
				l += fmt.Sprintf("dns lookup: %s", op.Name)
			} else {
				l += fmt.Sprintf("dns lookup: %s, timed out", op.Name)
			}
		default:
			l += fmt.Sprintf("%+v", p)
		}
	}

	m.message = l
	msgQueue <- m
}

func Start() {
	go func() {
		var count int
		var lastMsg *msg_t

		for {
		L:
			for {
				select {
				case m := <-msgQueue:
					if lastMsg != nil && m.dst != "" {
						if m.dst == lastMsg.dst {
							count++

							if count < 100 {
								continue L
							}
						}

						if count > 0 {
							fmt.Sprintln("[%d similar message(s)]", count)
						}
					}

					fmt.Println(m.message)
					lastMsg, count = &m, 0
				default:
					// nothing in queue to print, quit loop
					break L
				}
			}

			time.Sleep(200 * time.Millisecond)
		}
	}()
}

func D(params ...interface{}) {
	if logLevel <= -1 {
		print("D", params...)
	}
}

func L(params ...interface{}) {
	if logLevel <= 0 {
		print(" ", params...)
	}
}

func W(params ...interface{}) {
	if logLevel <= 1 {
		print("W", params...)
	}
}

func E(params ...interface{}) {
	if logLevel <= 2 {
		print("E", params...)
	}
}

func F(params ...interface{}) {
	print("X", params...)
	os.Exit(1)
}
