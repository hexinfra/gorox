Internal is the core of Hemi.

THE BIG PICTURE
===============

  TODO


THE WEB PICTURE
===============

```
  --http[1-3]Stream-->                        ----H[1-3]Stream--->
  --hweb[1-2]Stream-->                        ----B[1-2]Stream--->
                  ^                                           ^
  [REVISERS]      |    -------pass/post----->                 |
                  |                                           |
  webIn_          | webStream                 webOut_         | webStream
  ^ - stream -----+                           ^ - stream -----+
  |                                           |
  | - shell ------+                           | - shell ------+
  |               | webIn                     |               | webOut
  +-serverRequest_|                           +-clientRequest_|
    ^             |                             ^             |
    |             v                             |             v
    +-http[1-3]Request                          +-H[1-3]Request
    +-hweb[1-2]Request                          +-B[1-2]Request
                            \           /
                  \          \         /
        1/2/3      \          \       /             1/2/3
     [webServer]   [Handlet]/[webProxy]          [webClient]
        1/2/3      /          /       \             1/2/3
                  /          /         \
                            /           \
  <--http[1-3]Stream--                        <---H[1-3]Stream----
  <--hweb[1-2]Stream--                        <---B[1-2]Stream----
                   ^                                           ^
  [REVISERS]       |   <------pass/post------                  |
                   |                                           |
  webOut_          | webStream                webIn_           | webStream
  ^ - stream ------+                          ^ - stream ------+
  |                                           |
  | - shell -------+                          | - shell -------+
  |                | webOut                   |                | webIn
  +-serverResponse_|                          +-clientResponse_|
    ^              |                            ^              |
    |              v                            |              v
    +-http[1-3]Response                         +-H[1-3]Response
    +-hweb[1-2]Response                         +-B[1-2]Response
```


NOTES
-----

  * messages are composed of control, headers, [content, [trailers]].
  * control & headers is called head, and it must be small (<=16K).
  * contents, if exist (perhaps of zero size), may be large (>64K1) or small (<=64K1), sized or unsized.
  * trailers must be small (<=16K), and only exist when contents exist and are unsized.
  * incoming messages need parsing.
  * outgoing messages need building.
  * adding headers to incoming messages: apply + check.
  * adding headers to outgoing messages: insert + append.
  * deleting headers from outgoing messages: remove + delete.
  * proxies can be forward or reverse.
  * agents are reverse only.

WEB SERVER -> WEB PROXY -> WEB CLIENT
-------------------------------------

  * we support HTTP/1.x in server side, but we don't support HTTP/1.0 in client side.
  * we support revisers in server side, but we don't support revisers in client side.
  * HTTP/1.1 pipelining is recognized in server side, but not optimized.
  * HTTP/1.1 pipelining is not used in client side.
  * in HTTP/2 server side, streams are started passively (by receiving a HEADERS frame).
  * in HTTP/2 client side, streams are started actively.
