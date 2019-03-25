# istio-k8slite
Istio K8S to MCP adapter, using the light k8s client.

Goals:
- minimal overhead - the 'light' K8S client is only using protos and effectively has no 
abstraction layers, just helpers around consructing the http watcher and request.
- explore using the Pod info instead of Service.Endpoint, so Pilot gets the info faster
- maximum scalability - by reducing allocations, conversions, internal representations. 

The client library used (https://github.com/ericchiang/k8s) has auto-generated protos 
from k8s and directly uses the http client, with 'protobuf' content type. 

The code in this package will convert directly from the K8S proto to either 
ClusterLoadAssignment or ServiceEntry, and push to pilot as soon as possible, to 
minimize the startup latency.
