# How Computer Networks Work

## Layers of the internet

Network communication is organised in layers so that each layer can change without breaking the others. The TCP/IP model has four layers: the link layer, the internet layer, the transport layer and the application layer. The older OSI model describes the same ideas with seven layers. Each layer adds its own header to the data as it travels down the stack, a process called encapsulation.

## IP addresses

The Internet Protocol gives every device on a network an address so that packets can be routed to it. An IPv4 address is 32 bits long and is usually written as four numbers separated by dots, such as 192.168.1.10. Because IPv4 allows only about 4.3 billion addresses, the newer IPv6 standard uses 128-bit addresses written in hexadecimal. Routers read the destination address of each packet and forward it one hop closer to its destination.

## TCP and UDP

TCP is a connection-oriented protocol that guarantees data arrives complete and in order. Before sending data, TCP sets up a connection with a three-way handshake: the client sends SYN, the server replies with SYN-ACK, and the client answers with ACK. TCP numbers every byte, waits for acknowledgements and retransmits anything that is lost. UDP is connectionless and does not retransmit lost packets, which makes it faster and better suited to live video, voice calls and online games where a late packet is useless.

## Ports

A port number identifies which program on a device should receive the data, so one computer can run many network services at once. Port numbers range from 0 to 65535. Web servers listen on port 80 for HTTP and port 443 for HTTPS, while DNS uses port 53.

## DNS

The Domain Name System translates human-friendly names such as example.com into IP addresses. When you visit a website, your computer asks a recursive resolver, which queries the root servers, then the top-level domain servers, and finally the authoritative name server for the domain. Answers are cached for a period called the time to live, or TTL, so repeated lookups are fast.

## HTTP and HTTPS

HTTP is the protocol browsers use to request web pages. A request names a method such as GET to read a resource or POST to send data, and the server answers with a status code. Status code 200 means success, 404 means the resource was not found, and 500 means the server hit an error. HTTPS is HTTP sent over TLS, which encrypts the traffic and uses certificates so the browser can check it is talking to the real website.
