rc
==

make a bunch of concurrent HTTP requests


get it
======

	go get github.com/vbatts/rc


run it
======


	rc -url https://www.mydomain.com/content/some.file \
	  -cert=1359391926_4512.crt \
	  -key=1359391926_4512.key \
	  -requests=10000 \
	  -workers=50 \
	  -head \
	  -fail

This will make 10,000 requests to the url, over 50 workers. Including using the
provided client-side cert and key. It will only make HTTP HEAD requests, and
will quit on the first non-OK (HTTP 200) response.




enjoy
=====

take care,
vb
