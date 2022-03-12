# cligateway

An HTTP gateway to command line apps.

```shell
cligateway [cmd_whitelist ...]
```

Starts an HTTP service, converts requests into commands (in the `cmd_whitelist`), and responses running result.


## API

1. GET (Simple)

```http request
GET  /:command/*args[?flag=val]
```

run `$ command [--flag val] [args]`
and response `{"stdout": "", "stderr": ""}` in JSON or plain output (2>&1).

2. POST (more powerful)

```http request
POST /
data: {
    command: "cmd subcmd", 
    flags: {"k": "v"}, 
    args: ["arg"],
    envs: {"K": "V"}
}
```

will run `$ [K=V] command [--k v ] [a]`, for K, V in envs, for k, v in flags, and for a in args,
response `{"stdout": "", "stderr": ""}` in JSON or plain output text.

## Usage

```shell
Usage: cligateway [-h] [-add-dashes] [-env-key-to-upper] [-verbose] 
                  [-http ADDR] [-resp text|json] whitelist

positional arguments:

  whitelist `cmd1 cmd2 ...`
    	allowed commands. Make sure they are SAFE to expose.

optional arguments:

  -add-dashes
    	add dashes (-a or --word) to flags if not exist
  -env-key-to-upper
    	environment variables keys to UPPER
  -http string
    	HTTP service address. (default "localhost:8080")
  -resp string
    	text/json: response output in json {stdout, stderr} or plain text (default "json")
  -verbose
    	print verbose logs
```
