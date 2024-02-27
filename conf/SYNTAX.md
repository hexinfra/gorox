Comments
--------

  // This is a line comment.
  # This is a shell comment.
  /*
    We are in
	a stream comment.
  */

Values
------

  Booleans are like : true, false
  Strings are like  : "", "foo", "abc`def", `abc"def`
  Integers are like : 0, 314, 2222222222
  Sizes are like    : 1K, 2M, 3G, 4T
  Durations are like: 1s, 2m, 3h, 4d
  Lists are like    : (), ("a", "b"), ("c", 2, [])
  Dicts are like    : [], ["a" : 1, "b" : ("two")]

Properties
----------

  Component properties are prefixed a ".", like:

    .listen
    .maxSize

Constants
---------

  Predefined string constants are:

    %baseDir : Containing directory of the gorox executable
    %logsDir : Containing directory of the gorox log files
    %tmpsDir : Containing directory of the gorox temp files
    %varsDir : Containing directory of the gorox run-time data

Variables
---------

  Defined case variables are:

    $srcHost  : like "1.2.3.4", "[1::3]"
    $srcPort  : like "1234", "8888"
    $isUDS    : true or false
    $isTLS    : true or false
    $hostname : like "foobar.com"

  Defined rule variables are:

    $method      : like "GET", "POST"
    $scheme      : like "http", "https"
    $authority   : like "foo.com", "bar.com:8080"
    $hostname    : like "foo.com", "bar.com"
    $colonPort   : like ":80", ":8080"
    $path        : like "/foo"
    $uri         : like "/foo?bar=baz"
    $encodedPath : like "/%ab%cd"
    $queryString : like "?bar=baz"
    $contentType : like "application/json"

Comparisons
-----------

  Rule comparisons:

    ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
    -f, -d, -e, -D, -E, !f, !d, !e

  Case comparisons:

    ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~

References
----------

  You can refer to another property in one property, like:

    .abc = "hello, world"
    .def = .abc

  Here, property "def" has the same values as "abc".

String concatenation
--------------------

  Values of strings can be concatenated. For example:

    .abc = "world"
    .def = "hello," + " " + .abc + " " + %baseDir

  Here, property "def" is a concatenation of five string values.
