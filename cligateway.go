package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/gin-gonic/gin"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

var (
	addDashes     = flag.Bool("add-dashes", false, "add dashes (-a or --word) to flags if not exist")
	envKeyToUpper = flag.Bool("env-key-to-upper", false, "environment variables keys to UPPER")
	responseType  = flag.String("resp", "json", "text/json: response output in json {stdout, stderr} or plain text")
	serverAddr    = flag.String("http", "localhost:8080", "HTTP service address.")
	verboseRun    = flag.Bool("verbose", false, "print verbose logs")

	whitelist        = []string{"pwd"} // flag.Args() after flag.Parse()
	myRunner  runner = runJson         // runText or runJson
)

// request is a command:
//   [ENV=VAL] command [--flags vals] [args]
type request struct {
	command string `binding:"required"`
	flags   map[string]string
	args    []string
	envs    map[string]string
	// TODO: stdin
}

// flagStrings convert flags to strings:
//   map[string]string{"k0": "", "k1": "v1"} => []string{"--k0", "--k1", "v1"}
func (req request) flagStrings() []string {
	var flags []string
	for k, v := range req.flags {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		if *addDashes && !strings.HasPrefix(k, "-") { // to add - or --
			if len(k) == 1 {
				k = "-" + k
			} else {
				k = "--" + k
			}
		}

		flags = append(flags, k)
		if v != "" {
			flags = append(flags, v)
		}
	}
	return flags
}

// envStrings convert envs to strings:
//   req.envs map[string]string{"K": "V"} => []string{"K=V"}
func (req request) envStrings() []string {
	var envs []string
	for k, v := range req.envs {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)

		if *envKeyToUpper {
			k = strings.ToUpper(k)
		}
		envs = append(envs, k+"="+v)
	}
	return envs
}

// fullArgs = flagStrings + args
func (req request) fullArgs() []string {
	args := append(req.flagStrings(), req.args...)
	return args
}

// getHandler handle route "/:command/*args?flag=value"
// and run command:
//    command --flag value [args]
func getHandler(c *gin.Context) {
	var req request
	req.command = c.Param("command")
	req.args = strings.Split(c.Param("args"), "/")
	for k, v := range c.Request.URL.Query() {
		if k != "" && len(v) > 0 {
			req.flags[k] = v[0]
		}
	}
	run(c, req.command, req.fullArgs(), nil)
}

// postHandler parse requests like:
//   {command: "cmd subcmd", flags: {"k": "v"}, args: ["arg"], envs: {"K": "V"}}
// into a command, and run it:
//   [K=V for K, V in env] command [--k v for k, v in flags] [ args ]
//
// Example:
//   {"command": "pip install", "flags": {"upgrade": "", "i": "https://pypi.org/simple"}, "args": ["tensorflow"], envs: {"PYENV_VIRTUALENV_INIT": "1"}}
// Runs command:
//   PYENV_VIRTUALENV_INIT=1 pip install --upgrade -i https://pypi.org/simple tensorflow
func postHandler(c *gin.Context) {
	var req request
	if err := c.Bind(&req); err != nil {
		respError(c, http.StatusBadRequest, err.Error())
		return
	}

	var cmds = strings.Split(req.command, " ")
	if len(cmds) < 1 {
		respError(c, http.StatusBadRequest, "no command given")
	}
	command, subcmds := cmds[0], cmds[1:]

	args := append(subcmds, req.fullArgs()...)

	run(c, command, args, req.envStrings())
}

// inWhitelist check if command allowed
func inWhitelist(command string) bool {
	for _, w := range whitelist {
		if command == w {
			return true
		}
	}
	return false
}

func checkCommand(c *gin.Context, command string, args []string) {
	// check command
	if !inWhitelist(command) {
		if *verboseRun {
			log.Printf("run %v forbidden: not in whitelist, args=%#v", command, args)
		}
		respError(c, http.StatusForbidden, "command not allowed.")
		return
	}
}

// argsFilter remove empty args ("") and trim strings
func argsFilter(args []string) []string {
	var s []string
	for _, a := range args {
		if a == "" {
			continue
		}
		s = append(s, strings.TrimSpace(a))
	}
	return s
	// XXX: filter in place
	//    n := 0
	//    for _, a := range args {
	//        if a != "" {
	//            args[n] = a
	//            n++
	//        }
	//    }
}

// run a command, response stdout & stderr
func run(c *gin.Context, command string, args []string, env []string) {
	checkCommand(c, command, args)
	args = argsFilter(args)

	cmd := exec.CommandContext(c, command, args...)
	log.Printf("running %v: args=%#v", command, args)

	cmd.Env = env

	// run and response
	myRunner(c, cmd)
}

// runner run cmd and response result to c
type runner func(c *gin.Context, cmd *exec.Cmd)

// runText response the palin text output (2>&1)
func runText(c *gin.Context, cmd *exec.Cmd) {
	output, err := cmd.CombinedOutput()
	if err != nil {
		runFailed(c, cmd, err)
		return
	}
	if *verboseRun {
		log.Printf("run %v success: args=%#v, output=%v", cmd.Path, cmd.Args, string(output))
	}

	c.String(http.StatusOK, string(output))
}

// runJson response {"stdout": "...", "stderr": ""}
func runJson(c *gin.Context, cmd *exec.Cmd) {
	// stdout, stderr
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	// run and response
	if err := cmd.Run(); err != nil {
		runFailed(c, cmd, err)
		return
	}
	if *verboseRun {
		log.Printf("run %v success: args=%#v, stdout=%v, stderr=%v", cmd.Path, cmd.Args, outBuf.String(), errBuf.String())
	}
	c.JSON(http.StatusOK, gin.H{
		"stdout": outBuf.String(),
		"stderr": errBuf.String(),
	})
}

func runFailed(c *gin.Context, cmd *exec.Cmd, err error) {
	log.Printf("run %q failed: args=%#v error=%q", cmd.Path, cmd.Args, err.Error())
	respError(c, http.StatusInternalServerError, "failed to run cmd: "+err.Error())
}

func respError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{
		"error": message,
	})
}

func init() {
	usage := `
Usage: cligateway [-h] [-add-dashes] [-env-key-to-upper] [-verbose] 
                  [-http ADDR] [-resp text|json] whitelist

A HTTP gateway to command line apps.

Start a HTTP service, and handle routes:
    GET  /:command/*args[?flag=val]
       will run "$ command [--flag val] [args]" 
       and response {"stdout": "", "stderr": ""} in JSON or plain output (2>&1).
    POST /
       with data: {command: "cmd subcmd", flags: {"k": "v"}, 
                   args: ["arg"], envs: {"K": "V"}}
       will run "$ [K=V] command [--k v ] [a]", 
       for K, V in envs, for k, v in flags, for a in args,
       and response {"stdout": "", "stderr": ""} in JSON or plain output text.

`

	flag.Usage = func() {
		w := flag.CommandLine.Output()

		fmt.Fprintf(w, usage)
		fmt.Fprintf(w, "positional arguments:\n\n")
		fmt.Fprintf(w, "  whitelist `cmd1 cmd2 ...`\n"+
			"    \tallowed commands. Make sure they are SAFE to expose.\n\n")
		fmt.Fprintf(w, "optional arguments:\n\n")
		flag.PrintDefaults()

	}
}

func main() {
	flag.Parse()

	whitelist = flag.Args()
	if len(whitelist) == 0 {
		fmt.Fprintf(os.Stderr, "Empty command whitelist: nothing to do.\n")
		flag.Usage()
		os.Exit(1)
	}

	switch strings.ToLower(*responseType) {
	case "text":
		myRunner = runText
	default:
		*responseType = "json"
		myRunner = runJson
	}

	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	// TODO: timeout
	r.Use(gin.Logger(), gin.Recovery())

	r.GET("/:command/*args", getHandler)
	r.POST("/", postHandler)

	log.Printf("cligateway start:\n"+
		"\tListening and serving HTTP on %v\n"+
		"\tallowed commands: %v\n"+
		"\tresponse in %v", *serverAddr, whitelist, *responseType)

	log.Fatal(r.Run(*serverAddr))
}
