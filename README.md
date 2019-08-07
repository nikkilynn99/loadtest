# loadtest

The intention of the loadtest is to provide a command line utility that can make repeated queries against an http/https
URI.  The tool provides a number of parameters to make the test behave differently based on the needs of the test.

Arguments:
```
Usage: loadtest URL [options]
```

Without any options, the load test will constantly query the URL with a single thread.  Pressing Control-C will stop
the test and provide a list of responses from the server, ie:

```
./loadtest https://www.google.com --duration 10
Running 1 thread against: https://www.google.com (unlimited requests), maximum duration of 10 seconds
...............................................................................................................................................................................................................
0 errors of 207 requests (0% error rate) made in 10.033671 seconds.
20.63 requests/second.
Responses:
  200: 207 20.63 per second
```

## optional arguments

```
      --connect-timeout   Maximum time allowed for connection in seconds (default 3)
  -c, --count             The total number of requests to make. If count < 1 there is no limit (default 0)
  -d, --data              HTTP POST data
      --data-file         HTTP POST data from file
      --delay             Wait time between requests in seconds, per thread (default 0)
      --duration          The maximum duration for the test in seconds (default 0)
  -H, --header            Pass custom header(s) to server
  -h, --help              Display this help
  -k, --insecure          Allow insecure server connections when using SSL
  -t, --threads           How many concurrent threads to use in the load test (default 1)
  -v, --version           Show the version and exit
  -X, --command           Specify request command to use (default "GET")
```
