Comments
--------

  // This is a line comment.

  /* This is a stream comment.
   * It can span multiple lines.
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

References
----------

  You can refer to another property in one property, like:

    abc = "hello, world"
    def = abc

  Here, property "def" has the same values as "abc".

String concatenation
--------------------

  Values of strings can be concatenated. For example:

    abc = "world"
    def = "hello," + " " + abc + " " + @baseDir

  Here, property "def" is a concatenation of five string values.

Constants
---------

  Predefined string constants are:

    @baseDir : Containing directory of the gorox executable
    @dataDir : Containing directory of the gorox run-time datum
    @logsDir : Containing directory of the gorox log files
    @tempDir : Containing directory of the gorox run-time files

Variables
--------------------

  Defined rule variables are:

    %method
    %scheme
    %authority
    %hostname
    %colonPort
    %path
    %uri
    %encodedPath
    %queryString
    %contentType

  Defined case variables are:

    %srcHost
    %srcPort
    %transport
    %hostname

Comparisons
-----------

  Rule comparisons:

    ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
    -f, -d, -e, -D, -E, !f, !d, !e

  Case comparisons:

    ==, ^=

