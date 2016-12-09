[![Build Status](https://travis-ci.org/valyala/fastrpc.svg)](https://travis-ci.org/valyala/fastrpc)
[![GoDoc](https://godoc.org/github.com/valyala/fastrpc?status.svg)](http://godoc.org/github.com/valyala/fastrpc)
[![Go Report](https://goreportcard.com/badge/github.com/valyala/fastrpc)](https://goreportcard.com/report/github.com/valyala/fastrpc)


# fastrpc

Building blocks for fast rpc systems.


# Features

- Optimized for speed.
- Zero memory allocations in hot paths.
- Compression saves network bandwidth.

# How does it work?

It just sends batched rpc requests and responses over a single compressed
connection. This solves the following issues:

- High network bandwidth usage.
- High network packets rate.
- A lot of open TCP connections.


# Benchmark results

```
GOMAXPROCS=1 go test -bench=. -benchmem -benchtime=10s
BenchmarkEndToEndNoDelay1          	 5000000	      3491 ns/op	  74.75 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay10         	 5000000	      3540 ns/op	  73.73 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay100        	 5000000	      3546 ns/op	  73.60 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay1000       	 5000000	      3544 ns/op	  73.63 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay10K        	 3000000	      4169 ns/op	  62.60 MB/s	       7 B/op	       0 allocs/op
BenchmarkEndToEndDelay1ms          	 3000000	      3702 ns/op	  70.48 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndDelay2ms          	 3000000	      4991 ns/op	  52.29 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndDelay4ms          	 2000000	      6999 ns/op	  37.29 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndDelay8ms          	 1000000	     10952 ns/op	  23.83 MB/s	       3 B/op	       0 allocs/op
BenchmarkEndToEndDelay16ms         	 1000000	     18662 ns/op	  13.99 MB/s	       3 B/op	       0 allocs/op
BenchmarkEndToEndCompressNone      	 3000000	      3982 ns/op	  65.53 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndCompressFlate     	 2000000	      8718 ns/op	  29.94 MB/s	       5 B/op	       0 allocs/op
BenchmarkEndToEndCompressSnappy    	 3000000	      4013 ns/op	  65.04 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressNone   	 3000000	      3948 ns/op	  66.09 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressFlate  	 2000000	      8112 ns/op	  32.17 MB/s	       5 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressSnappy 	 3000000	      4717 ns/op	  55.32 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndPipeline1         	 5000000	      3359 ns/op	  77.69 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndPipeline10        	 5000000	      3343 ns/op	  78.07 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndPipeline100       	 5000000	      3364 ns/op	  77.58 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndPipeline1000      	 5000000	      3330 ns/op	  78.37 MB/s	       0 B/op	       0 allocs/op
```

# Users

- [httpteleport](https://github.com/valyala/httpteleport)
