Rperf is a simple HTTP benchmarking tool written in Rust.

shell> rperf -c 128 -r 10000 -u http1://1.2.3.4:3080/path?query
shell> rperf -c 128 -r 10000 -u http1s://1.2.3.4:3443/path?query

shell> rperf -c 128 -r 10000 -s 100 -u http2://1.2.3.4:3080/path?query
shell> rperf -c 128 -r 10000 -s 100 -u http2s://1.2.3.4:3443/path?query

shell> rperf -c 128 -r 10000 -s 100 -u http3://1.2.3.4:3443/path?query
