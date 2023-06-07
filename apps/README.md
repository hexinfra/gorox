This directory holds your Web applications in gorox, generally one app one
directory, but many related apps can be grouped into a container directory.

Some builtin apps are:

  examples/chinese/    - Chinese version of official website.
  examples/english/    - English version of official website.
  examples/forums/     - An example forums app.
  examples/hello/      - An example "hello, world" app.

To add a new static app named "foo":

  1. Create a folder called "foo" in this directory,
  2. Create its config file "foo/app.conf" and configure it correctly,
  3. Put your static files in "foo/",
  4. Add app "foo" in "../conf/gorox.conf" and bind it to your web servers.

To add a new Go app named "bar":

  1. Create a folder called "bar" in this directory,
  2. Create its config file "bar/app.conf" and configure it correctly,
  3. Create Go file "bar/goapp.go" with initial code,
  4. Import package "bar" in "import.go",
  5. Add app "bar" in "../conf/gorox.conf" and bind it to your web servers.

You can also put your PHP apps, Python apps, and so on in this directory.

In gorox, a web server (i.e. httpxServer, http3Server) can host many apps,
whereas an app can be bound to many web servers.

For examples, see builtin apps.
