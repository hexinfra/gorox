v?.?.?                                                        (2023-??-?? UTC+8)
================================================================================

  * Interface of TCPSFilter is refactored.
  * Storers are renamed as cachers.
  * Fix crash caused by empty value of some fields.
  * Flag "-temp" changed to "-tmps".
  * procman.Main() now accepts an args parameter.
  * Action "test" is not integrated.
  * Values can now be variables in config file.
  * HTTP proxies now support adding and deleting request headers.
  * ajpProxy, fcgiProxy, and uwsgiProxy are now renamed as respective relays.
  * FromFile and FromText are renamed as BootFile and BootText respectively.
  * Abbreviations are now differentiated from names clearly.
  * Directory "cmds/" is now renamed as "bins/".

v0.1.7                                                        (2023-06-13 UTC+8)
================================================================================

  * Max debug level is now 0-3.
  * hemi.IsDebug() is changed to hemi.Debug().
  * Fix FCGI bug that large content is truncated.
  * Editor support in mesher is removed.
  * Dealer is renamed as Filter in mesher.

v0.1.6                                                        (2023-06-06 UTC+8)
================================================================================

  * Conn and Conn_ are now exported.
  * Cachers are renamed as storers.
  * AJP, FCGI, and uwsgi now support unix domain sockets.
  * Add "contains" and "not contains" support for cases and rules.
  * Add regexp conditions in cases and rules.
  * Option "-out" and "-err" are changed to "-stdout" and "-stderr".
  * hemi/website is merged into Gorox.
  * Myrox is removed from Gorox.

v0.1.5                                                        (2023-06-02 UTC+8)
================================================================================

  * hemi/gosites is now renamed as hemi/website.
  * Added quds, tuds, and uuds support.
  * Routers are renamed as meshers.
  * Mappers are renamed as routers.
  * hemi/internal is reorganized entirely.

v0.1.4                                                        (2023-05-22 UTC+8)
================================================================================

  * Fix panic when some fields are mixed with values and params.
  * Log more info when worker is broken.

v0.1.3                                                        (2023-05-16 UTC+8)
================================================================================

  * Optimize actions and options.
  * Introduce WebUI in leader.

v0.1.2                                                        (2023-05-10 UTC+8)
================================================================================

  * Various improvement.

v0.1.1                                                        (2023-05-01 UTC+8)
================================================================================

  * Various improvement.

v0.1.0                                                        (2022-10-21 UTC+8)
================================================================================

  * The first public version.
