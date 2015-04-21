package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/howeyc/gopass" // for reading password (setty off)
	"github.com/souravdatta/goftp"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	Timeout = 5 * 60 // timeout in seconds
)

// Command processing
// Commands
const (
	BAD int = iota
	PWD     // list present working directory
	CD      // cd to directory
	LS      // simple list files in current directory
	PUT     // put a file
	GET     // get a file
)

func get_command(part string) int {
	switch part {
	case "pwd":
		return PWD
	case "cd":
		return CD
	case "ls":
		return LS
	case "put":
		return PUT
	case "get":
		return GET
	}

	return BAD
}

func parse(command string) (code int, arg string) {
	parts := strings.Split(command, " ")
	if len(parts) == 1 {
		code = get_command(parts[0])
		arg = ""
	} else if len(parts) > 1 {
		code = get_command(parts[0])

		if code == PUT || code == GET {
			arg = strings.Join(parts[1:len(parts)], " ")
		} else {
			arg = parts[1]
		}
	}
	return
}

// parse end

// Expiry timer
type command_timer struct {
	tmr       *time.Timer
	is_active bool
	duration  time.Duration
}

func (ctm *command_timer) init(d int) {
	ctm.is_active = false
	ctm.duration = time.Duration(d) * time.Second
}

func (ctm *command_timer) reset() {
	if ctm.is_active == true {
		ctm.tmr.Reset(ctm.duration)
		return
	}
	ctm.is_active = true
	end_fn := func() {
		ctm.is_active = false
	}
	ctm.tmr = time.AfterFunc(ctm.duration, end_fn)
}

// timer

// The global context definition
type context struct {
	server_name string
	user_name   string
	password    string
	last_dir    string
	ftp         *goftp.FTP
	debug       bool
	timer       command_timer
}

func (ctx *context) init_context(sn, un, pass string, dbg bool) {
	ctx.server_name = sn
	ctx.user_name = un
	ctx.password = pass
	ctx.last_dir = ""
	ctx.ftp = nil
	ctx.debug = dbg

	ctx.timer.init(Timeout)
}

func (ctx *context) disconnect() {
	ctx.ftp.Close()
}

func (ctx *context) connect() {
	var err error
	// Connect to server
	if ctx.debug {
		ctx.ftp, err = goftp.ConnectDbg(ctx.server_name + ":21")
	} else {
		ctx.ftp, err = goftp.Connect(ctx.server_name + ":21")
	}
	if err != nil {
		panic(err)
	}
	// Login
	err = ctx.ftp.Login(ctx.user_name, ctx.password)
	if err != nil {
		// Sorry, but this is not working!
		fmt.Printf("Cannot login to server...\n")
		panic(err)
	}

	err = nil
	// Set last working directory if required
	if ctx.last_dir != "" {
		// Chdir to last working directory
		ctx.ftp.Cwd(ctx.last_dir)
	} else {
		// Get current working directory
		// TODO to update on a Chdir call
		ctx.last_dir, err = ctx.ftp.Pwd()
		if err != nil {
			panic(err)
		}
	}
}

func (ctx *context) action(code int, arg string) string {
	switch code {
	case BAD:
		return "Don't know that!"
	case PWD:
		{
			return ctx.last_dir
		}
	case CD:
		{
			if arg == "" {
				arg = "~"
			}

			err := ctx.ftp.Cwd(arg)
			if err != nil {
				return err.Error()
			} else {
				ctx.last_dir, err = ctx.ftp.Pwd()
				if err != nil {
					panic(err)
				}
			}
		}
	case LS:
		{
			if arg == "" {
				arg = "."
			}
			files, err := ctx.ftp.List(arg)
			if err != nil {
				return err.Error()
			} else {
				return strings.Join(files, "")
			}
		}
	case PUT:
		{
			if arg == "" {
				return "No arguments"
			}

			pargs := strings.Split(arg, " ")

			for _, parg := range pargs {
				var files []string
				var err error
				var rd io.Reader

				if parg == "" {
					return "Specify files to be PUT"
				}

				if files, err = filepath.Glob(parg); err != nil {
					return err.Error()
				}

				for _, f := range files {
					if rd, err = os.Open(f); err != nil {
						return err.Error()
					}
					fmt.Println("\tputting " + f)
					if err := ctx.ftp.Stor(f, rd); err != nil {
						return err.Error()
					}
				}
			}
		}
	case GET:
		{
			if arg == " " {
				return "No arguments"
			}

			pargs := strings.Split(arg, " ")

			for _, parg := range pargs {
				files, err := ctx.ftp.List(parg)
				if err != nil {
					return err.Error()
				}
				for _, f := range files {
					parts := strings.Split(strings.Trim(f, "\n\r"), " ")
					p := parts[len(parts)-1]
					fmt.Println("\tgetting " + p)
					ctx.ftp.Retr(p, func(f io.Reader) error {
						content, err := ioutil.ReadAll(f)
						if err != nil {
							panic(err)
						}
						if err = ioutil.WriteFile(p, content, os.ModePerm); err != nil {
							return err
						}

						return nil
					})
				}
			}
		}
	}

	return ""
}

// End context

// Util functions
// Usage ~ print usage info to Stdout
func usage() {
	fmt.Printf("Usage: %s -server <server-name> [-user <use-name> [-passwd <passwd>]] [-debug true/false]\n", os.Args[0])
	os.Exit(1)
}

// read_credentials ~ read user name and password
func read_username(user_name *string) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("username: ")
	*user_name, _ = reader.ReadString('\n')
	*user_name = strings.Trim(*user_name, "\n\r")
}

func read_password(passwd *string) {
	fmt.Printf("password: ")
	*passwd = string(gopass.GetPasswd())
}

// The session REPL
func repl() {
	var command string
	var err error

	rd := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("ftps[" + ctx.last_dir + "]$ ")
		command, err = rd.ReadString('\n')

		if err != nil {
			break
		}

		command = strings.Trim(command, "\n\r")

		if command == "quit" || command == "exit" {
			break
		}

		code, arg := parse(command)

		if !ctx.timer.is_active {
			ctx.disconnect()
			ctx.connect()
		}

		fmt.Println(ctx.action(code, arg))
		ctx.timer.reset()
	}
}

// End util functions

// The global context
var ctx context

// main ~ Ah the main!
func main() {
	// Create the global context
	ctx = context{}

	// Handle command line args
	user_name := flag.String("user", "", "user name (default anonymous)")
	password := flag.String("passwd", "", "password (default blank)")
	server_name := flag.String("server", "", "Server name")
	debug_mode := flag.Bool("debug", false, "Debugging on")

	flag.Parse()

	if *server_name == "" {
		usage()
	} else if *user_name == "" {
		if *password != "" {
			usage()
		}
		read_username(user_name)
		read_password(password)
	} else if *password == "" {
		read_password(password)
	}

	ctx.init_context(*server_name, *user_name, *password, *debug_mode)
	// Enough fun with command line parsing, now to work

	log.Printf("connecting ~ (%s/****@%s)\n", ctx.user_name, ctx.server_name)
	ctx.connect()
	log.Printf("connected")

	// Fire up the REPL
	ctx.timer.reset()
	repl()

	// Done work
	ctx.disconnect()
	log.Print("exit")
}
