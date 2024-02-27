IETF QUIC Implementation.
See RFC 8999-9002.

What TLS 1.3 library should we use?

Both UDP and UDS(unixgram) are supported.
SO_REUSEPORT is a must for server.
recvmmsg(2), sendmmsg(2) is a plus.
UDP GSO is a plus.
