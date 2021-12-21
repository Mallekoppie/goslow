# Overview
This example shows how a client token can be forwarded to a remote API

# Setup
Setup IDP clients for the client and server services

# Process
* Dialer gets token for client service
* Dialer calls client service
* Client service validates token
* Client service gets own token
* Client service calls server service and attaches the client token
* Server service validates token and then also validates the original client token
* Server service can now do authorization based on original client token