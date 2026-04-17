package istio

const (
	EgressNamespace        = "istio-egress"
	gatewayHost            = "istio-egressgateway.istio-egress.svc.cluster.local"
	gatewayName            = "istio-egressgateway"
	meshGatewayName        = "mesh"
	istioGatewaySelector   = "istioegressgateway"
	egressToGatewayLabel   = "egress-to-gateway"
	egressFromGatewayLabel = "egress-from-gateway"
	httpsProtocol          = "HTTPS"
	tcpProtocol            = "TCP"
	hostTypeLabelExternal  = "external"
	hostTypeLabelInternal  = "internal"
)
