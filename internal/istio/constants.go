package istio

const (
	EgressNamespace        = "istio-egress"
	externalGatewayHost    = "istio-egressgateway-external.istio-egress.svc.cluster.local"
	externalGatewayName    = "istio-egressgateway-external"
	cloudSQLGatewayHost    = "istio-egressgateway-cloudsql.istio-egress.svc.cluster.local"
	cloudSQLGatewayName    = "istio-egressgateway-cloudsql"
	meshGatewayName        = "mesh"
	istioGatewaySelector   = "egressgateway"
	egressToGatewayLabel   = "egress-to-gateway"
	egressFromGatewayLabel = "egress-from-gateway"
	httpsProtocol          = "HTTPS"
	tcpProtocol            = "TCP"
	tlsProtocol            = "TLS"
	hostTypeLabelExternal  = "external"
	hostTypeLabelInternal  = "internal"
	managedByLabel         = "app.kubernetes.io/managed-by"
	managedByValue         = "union-operator"
	httpPort               = 80
	httpsPort              = 443
	cloudSQLPort           = 3307
)
