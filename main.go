package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger is a simple log handler, out puts in the standard of apache access log common.
// See http://httpd.apache.org/docs/2.2/logs.html#accesslog
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		t := time.Now()
		ip, err := net.ResolveTCPAddr("tcp", c.Request.RemoteAddr)
		if err != nil {
			c.Abort()
		}

		// before request
		c.Next()
		// after request

		user := "-"
		if c.Request.URL.User != nil {
			user = c.Request.URL.User.Username()
		}

		latency := time.Since(t)

		// This is the format of Apache Log Common, with an additional field of latency
		fmt.Printf("%v - %v [%v] \"%v %v %v\" %v %v %v\n",
			ip.IP, user, t.Format(time.RFC3339), c.Request.Method, c.Request.URL.Path,
			c.Request.Proto, c.Writer.Status(), c.Request.ContentLength, latency)
	}
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func mainHandler(c *gin.Context) {
	fields := strings.Split(c.Params.ByName("field"), ".")
	ip, err := net.ResolveTCPAddr("tcp", c.Request.RemoteAddr)
	if err != nil {
		c.Abort()
	}

	cfIP := net.ParseIP(c.Request.Header.Get("CF-Connecting-IP"))
	if cfIP != nil {
		ip.IP = cfIP
	}

	c.Set("ip", ip.IP.String())
	c.Set("port", ip.Port)
	c.Set("ua", c.Request.UserAgent())
	c.Set("lang", c.Request.Header.Get("Accept-Language"))
	c.Set("encoding", c.Request.Header.Get("Accept-Encoding"))
	c.Set("method", c.Request.Method)
	c.Set("mime", c.Request.Header.Get("Accept"))
	c.Set("referer", c.Request.Header.Get("Referer"))
	c.Set("forwarded", c.Request.Header.Get("X-Forwarded-For"))
	c.Set("country_code", c.Request.Header.Get("CF-IPCountry"))

	// Only lookup hostname if the results are going to need it.
	if stringInSlice(fields[0], []string{"", "all", "host"}) {
		hostnames, err := net.LookupAddr(ip.IP.String())
		if err != nil {
			c.Set("host", "")
		} else {
			c.Set("host", hostnames[0])
		}
	}

	wantsJSON := false
	if len(fields) >= 2 && fields[1] == "json" {
		wantsJSON = true
	}

	ua := strings.Split(c.Request.UserAgent(), "/")
	switch fields[0] {
	case "":
		//If the user is using curl, then we should just return the IP, else we show the home page.
		if ua[0] == "curl" {
			c.String(200, fmt.Sprintln(ip.IP))
		} else {
			c.HTML(200, "index.html", c.Keys)
		}
		return
	case "request":
		c.JSON(200, c.Request)
		return
	case "all":
		if wantsJSON {
			c.JSON(200, c.Keys)
		} else {
			c.String(200, "%v", c.Keys)
		}
		return
	}

	fieldResult, err := c.Get(fields[0])
	if err != nil {
		c.String(404, "Not Found")
	}
	c.String(200, fmt.Sprintln(fieldResult))

}

// FileServer is a basic file serve handler, this is just here as an example.
// gin.Static() should be used instead
func FileServer(root string) gin.HandlerFunc {
	return func(c *gin.Context) {
		file := c.Params.ByName("file")
		if !strings.HasPrefix(file, "/") {
			file = "/" + file
		}
		http.ServeFile(c.Writer, c.Request, path.Join(root, path.Clean(file)))
	}
}

func main() {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(Logger())
	r.LoadHTMLTemplates("templates/*")

	r.GET("/:field", mainHandler)
	r.GET("/", mainHandler)

	// Create a listener for FCGI
	fcgi_listen, err := net.Listen("tcp", "127.0.0.1:4000")
	if err != nil {
		panic(err)
	}
	errc := make(chan error)
	go func(errc chan error) {
		for err := range errc {
			panic(err)
		}
	}(errc)

	go func(errc chan error) {
		errc <- fcgi.Serve(fcgi_listen, r)
	}(errc)

	port := os.Getenv("PORT")
	host := os.Getenv("HOST")
	if port == "" {
		port = "8080"
	}
	errc <- r.Run(host + ":" + port)
}
