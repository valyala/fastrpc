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
GOMAXPROCS=1 go test -bench=. -benchmem
goos: linux
goarch: amd64
pkg: github.com/valyala/fastrpc
BenchmarkCoarseTimeNow             	500000000	         3.91 ns/op	       0 B/op	       0 allocs/op
BenchmarkTimeNow                   	30000000	        47.1 ns/op	       0 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay1          	  500000	      3442 ns/op	  75.81 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay10         	  500000	      3440 ns/op	  75.85 MB/s	       2 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay100        	  500000	      3448 ns/op	  75.69 MB/s	       2 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay1000       	  500000	      3440 ns/op	  75.86 MB/s	       8 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay10K        	  300000	      3960 ns/op	  65.90 MB/s	      79 B/op	       0 allocs/op
BenchmarkEndToEndDelay1ms          	  500000	      3395 ns/op	  76.87 MB/s	       6 B/op	       0 allocs/op
BenchmarkEndToEndDelay2ms          	  500000	      3519 ns/op	  74.16 MB/s	       6 B/op	       0 allocs/op
BenchmarkEndToEndDelay4ms          	  300000	      4562 ns/op	  57.20 MB/s	      10 B/op	       0 allocs/op
BenchmarkEndToEndDelay8ms          	  200000	      8909 ns/op	  29.30 MB/s	      15 B/op	       0 allocs/op
BenchmarkEndToEndDelay16ms         	  100000	     16647 ns/op	  15.68 MB/s	      31 B/op	       0 allocs/op
BenchmarkEndToEndCompressNone      	  500000	      3376 ns/op	  77.29 MB/s	       6 B/op	       0 allocs/op
BenchmarkEndToEndCompressFlate     	  200000	      8154 ns/op	  32.01 MB/s	      49 B/op	       0 allocs/op
BenchmarkEndToEndCompressSnappy    	  500000	      3665 ns/op	  71.20 MB/s	      10 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressNone   	  300000	      3892 ns/op	  67.06 MB/s	      15 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressFlate  	  200000	      8474 ns/op	  30.80 MB/s	      51 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressSnappy 	  300000	      3851 ns/op	  67.77 MB/s	      18 B/op	       0 allocs/op
BenchmarkEndToEndPipeline1         	  500000	      3249 ns/op	  80.31 MB/s	       0 B/op	       0 allocs/op
BenchmarkEndToEndPipeline10        	  500000	      3232 ns/op	  80.75 MB/s	       2 B/op	       0 allocs/op
BenchmarkEndToEndPipeline100       	  500000	      3237 ns/op	  80.62 MB/s	       2 B/op	       0 allocs/op
BenchmarkEndToEndPipeline1000      	  500000	      3215 ns/op	  81.17 MB/s	       8 B/op	       0 allocs/op
BenchmarkSendNowait                	 3000000	       451 ns/op	       0 B/op	       0 allocs/op

GOMAXPROCS=4 go test -bench=. -benchmem
goos: linux
goarch: amd64
pkg: github.com/valyala/fastrpc
BenchmarkCoarseTimeNow-4               	1000000000	         2.37 ns/op	       0 B/op	       0 allocs/op
BenchmarkTimeNow-4                     	100000000	        17.7 ns/op	       0 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay1-4            	 1000000	      1726 ns/op	 151.14 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay10-4           	 1000000	      1757 ns/op	 148.47 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay100-4          	 1000000	      1768 ns/op	 147.59 MB/s	       2 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay1000-4         	 1000000	      1610 ns/op	 162.09 MB/s	      11 B/op	       0 allocs/op
BenchmarkEndToEndNoDelay10K-4          	 1000000	      1825 ns/op	 142.94 MB/s	      41 B/op	       0 allocs/op
BenchmarkEndToEndDelay1ms-4            	 1000000	      1656 ns/op	 157.57 MB/s	       8 B/op	       0 allocs/op
BenchmarkEndToEndDelay2ms-4            	 1000000	      1715 ns/op	 152.10 MB/s	       8 B/op	       0 allocs/op
BenchmarkEndToEndDelay4ms-4            	 1000000	      1635 ns/op	 159.62 MB/s	       7 B/op	       0 allocs/op
BenchmarkEndToEndDelay8ms-4            	  500000	      2322 ns/op	 112.36 MB/s	      15 B/op	       0 allocs/op
BenchmarkEndToEndDelay16ms-4           	  300000	      4944 ns/op	  52.78 MB/s	      27 B/op	       0 allocs/op
BenchmarkEndToEndCompressNone-4        	 1000000	      1688 ns/op	 154.58 MB/s	       8 B/op	       0 allocs/op
BenchmarkEndToEndCompressFlate-4       	  500000	      3129 ns/op	  83.40 MB/s	      29 B/op	       0 allocs/op
BenchmarkEndToEndCompressSnappy-4      	 1000000	      1720 ns/op	 151.68 MB/s	       9 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressNone-4     	 1000000	      1780 ns/op	 146.58 MB/s	       9 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressFlate-4    	  500000	      3133 ns/op	  83.29 MB/s	      29 B/op	       0 allocs/op
BenchmarkEndToEndTLSCompressSnappy-4   	 1000000	      1758 ns/op	 148.45 MB/s	      10 B/op	       0 allocs/op
BenchmarkEndToEndPipeline1-4           	 1000000	      1638 ns/op	 159.27 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndPipeline10-4          	 1000000	      1646 ns/op	 158.56 MB/s	       1 B/op	       0 allocs/op
BenchmarkEndToEndPipeline100-4         	 1000000	      1623 ns/op	 160.78 MB/s	       2 B/op	       0 allocs/op
BenchmarkEndToEndPipeline1000-4        	 1000000	      1386 ns/op	 188.20 MB/s	      10 B/op	       0 allocs/op
BenchmarkSendNowait-4                  	 5000000	       327 ns/op	       1 B/op	       0 allocs/op
```

# Users

- [httpteleport](https://github.com/valyala/httpteleport)
