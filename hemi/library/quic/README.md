IETF QUIC Implementation. See RFC 8999, RFC 9000, RFC 9001, and RFC 9002.

import:
	net
	crypto/tls

Both UDP and UDS(unixgram) are supported.
SO_REUSEPORT is a must for UDP server.
recvmmsg(2), sendmmsg(2) is a plus.
UDP GSO is a plus.
