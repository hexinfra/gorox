This directory holds your Web applications in gorox, generally one webapp one
directory, but many related webapps can be grouped into a container directory.

To add a new static or configured webapp named "foo":

  1. Create a folder called "foo" in this directory,
  2. Create its config file "foo/webapp.conf" and configure it correctly,
  3. Put your static files (HTML and CSS files, etc) in "foo/web/",
  4. Add webapp "foo" in "../conf/gorox.conf" and bind it to your web servers.

To add a new Go webapp named "bar":

  1. Create a folder called "bar" in this directory,
  2. Create its config file "bar/webapp.conf" and configure it correctly,
  3. Create Go file "bar/webapp.go" with initial code,
  4. Import package "bar" in "import.go",
  5. Add webapp "bar" in "../conf/gorox.conf" and bind it to your web servers.

For convenience, you can also put your PHP webapps, Python webapps, and so on
under this directory.

In gorox, a web server (i.e. httpxServer, http3Server) can host many webapps,
whereas a webapp can be bound to many web servers.

For examples, see "examples/".
