package istio

const (
	EgressNamespace        = "istio-egress"
	gatewayHost            = "istio-egressgateway-external.istio-egress.svc.cluster.local"
	gatewayName            = "istio-egressgateway-external"
	meshGatewayName        = "mesh"
	istioGatewaySelector   = "egressgateway"
	egressToGatewayLabel   = "egress-to-gateway"
	egressFromGatewayLabel = "egress-from-gateway"
	httpsProtocol          = "HTTPS"
	tcpProtocol            = "TCP"
	hostTypeLabelExternal  = "external"
	hostTypeLabelInternal  = "internal"
)
