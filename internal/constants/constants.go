package constants

import "net"

/* =========================================================
 * RADIUS FIXED FIELD SIZES
 * ========================================================= */

const (
	RadiusLengthFieldLength      = 2
	RadiusIdentifierFieldLength  = 1
	RadiusAuthenticatorFieldLen  = 16
	RadiusHdrLen                 = 20
)

/* =========================================================
 * ROUTING KEYS
 * ========================================================= */

const (
	RKSessionStart      = "session.start"
	RKSessionStop       = "session.stop"
	RKSessionStats      = "session.stats"
	RKSyncSession       = "sync.session"
	RKSyncSessionDelete = "sync.session.delete"
)

/* =========================================================
 * RADIUS ATTRIBUTE TYPES (RFC MAPPING)
 * ========================================================= */

const (
	UserName                      = 1
	UserPassword                  = 2
	ChapPassword                  = 3
	NasIPAddress                 = 4
	NasPort                      = 5
	ServiceType                  = 6
	FramedProtocol               = 7
	FramedIPAddress              = 8
	FramedIPNetmask              = 9
	FramedRouting                = 10
	FilterID                     = 11
	FramedMTU                    = 12
	FramedCompression           = 13
	LoginIPHost                 = 14
	LoginService                = 15
	LoginTCPPort                = 16
	ReplyMessage                = 18
	CallbackNumber              = 19
	CallbackID                  = 20
	FramedRoute                 = 22
	FramedIPXNetwork            = 23
	State                       = 24
	Class                       = 25
	VendorSpecific              = 26
	SessionTimeout             = 27
	IdleTimeout                = 28
	TerminationAction          = 29
	CalledStationID           = 30
	CallingStationID          = 31
	NasIdentifier             = 32
	ProxyState               = 33
	LoginLatService         = 34
	LoginLatNode            = 35
	LoginLatGroup           = 36
	FramedAppletalkLink     = 37
	FramedAppletalkNetwork  = 38
	FramedAppletalkZone     = 39
	AcctStatusType          = 40
	AcctDelayTime           = 41
	AcctInputOctets         = 42
	AcctOutputOctets        = 43
	AcctSessionID           = 44
	AcctAuthentic           = 45
	AcctSessionTime         = 46
	AcctInputPackets        = 47
	AcctOutputPackets       = 48
	AcctTerminateCause      = 49
	AcctMultiSessionID      = 50
	AcctLinkCount           = 51
	AcctInputGigaWords      = 52
	AcctOutputGigaWords     = 53
	EventTimestamp          = 55
	EgressVLANID            = 56
	IngressFilters          = 57
	EgressVLANName          = 58
	UserPriorityTable       = 59
	ChapChallenge           = 60
	NasPortType             = 61
	PortLimit               = 62
	LoginLatPort            = 63
	TunnelType              = 64
	TunnelMediumType        = 65
	TunnelClientEndpoint    = 66
	TunnelServerEndpoint    = 67
	AcctTunnelConnection    = 68
	TunnelPassword          = 69
	ArapPassword            = 70
	ArapFeatures            = 71
	ArapZoneAccess          = 72
	ArapSecurity            = 73
	ArapSecurityData        = 74
	PasswordRetry           = 75
	Prompt                  = 76
	ConnectInfo             = 77
	ConfigurationToken      = 78
	EAPMessage              = 79
	MessageAuthenticator    = 80
	TunnelPrivateGroupID    = 81
	TunnelAssignmentID      = 82
	TunnelPreference        = 83
	ArapChallengeResponse   = 84
	AcctInterimInterval     = 85
	AcctTunnelPacketsLost   = 86
	NasPortID               = 87
	FramedPool              = 88
	CUI                     = 89
	TunnelClientAuthID      = 90
	TunnelServerAuthID      = 91
	NasFilterRule           = 92
	OriginatingLineInfo     = 94
	NasIPv6Address          = 95
	FramedInterfaceID       = 96
	FramedIPv6Prefix        = 97
	LoginIPv6Host           = 98
	FramedIPv6Route         = 99
	FramedIPv6Pool          = 100
	ErrorCause              = 101
	EAPKeyName             = 102
	DigestResponse         = 103
	DigestRealm            = 104
	DigestNonce            = 105
	DigestResponseAuth     = 106
	DigestNextNonce        = 107
	DigestMethod           = 108
	DigestURI              = 109
	DigestQOP              = 110
	DigestAlgorithm        = 111
	DigestEntityBodyHash   = 112
	DigestCNonce           = 113
	DigestNonceCount       = 114
	DigestUsername         = 115
	DigestOpaque           = 116
	DigestAuthParam        = 117
	DigestAkaAUTS          = 118
	DigestDomain           = 119
	DigestStale            = 120
	DigestHA1              = 121
	SIPAOR                 = 122
	DelegatedIPv6Prefix    = 123
	MIP6FeatureVector      = 124
	MIP6HomeLinkPrefix     = 125
	OperatorName           = 126
	LocationInformation    = 127
	LocationData           = 128
	BasicLocationPolicyRules = 129
	ExtendedLocationPolicyRules = 130
	LocationCapable        = 131
	RequestedLocationInfo  = 132
	FramedManagementProtocol = 133
	ManagementTransportProtection = 134
	ManagementPolicyID     = 135
	ManagementPrivilegeLevel = 136
	PKMSscCert             = 137
	PKMcaCert              = 138
	PKMConfigSettings      = 139
	PKMCryptoSuiteList     = 140
	PKMSAID                = 141
	PKMSADescriptor        = 142
	PKMAuthKey             = 143
	DSLiteTunnelName       = 144
	MobileNodeIdentifier   = 145
	ServiceSelection       = 146
	PMIP6HomeLMAIPv6Addr   = 147
	PMIP6VisitedLMAIPv6Addr = 148
	PMIP6HomeLMAIPv4Addr   = 149
	PMIP6VisitedLMAIPv4Addr = 150
	PMIP6HomeHNPrefix      = 151
	PMIP6VisitedHNPrefix   = 152
	PMIP6HomeInterfaceID   = 153
	PMIP6VisitedInterfaceID = 154
	PMIP6HomeIPv4HOA       = 155
	PMIP6VisitedIPv4HOA    = 156
	PMIP6HomeDHCP4Server   = 157
	PMIP6VisitedDHCP4Server = 158
	PMIP6HomeDHCP6Server   = 159
	PMIP6VisitedDHCP6Server = 160
	PMIP6HomeIPv4Gateway   = 161
	PMIP6VisitedIPv4Gateway = 162
	EAPLowerLayer          = 163
	GSSAcceptorServiceName = 164
	GSSAcceptorHostName    = 165
	GSSAcceptorServiceSpecifics = 166
	GSSAcceptorRealmName   = 167
	FramedIPv6Address      = 168
	DNSServerIPv6Address   = 169
	RouteIPv6Information   = 170
	DelegatedIPv6PrefixPool = 171
	StatefulIPv6AddressPool = 172
	IPv66RDConfiguration   = 173
	AllowedCalledStationID = 174
	EAPPeerID              = 175
	EAPServerID            = 176
	MobilityDomainID       = 177
	PreauthTimeout         = 178
	NetworkIDName          = 179
	EAPOLAnnouncement      = 180
	WLANHESSID             = 181
	WLANVenueInfo          = 182
	WLANVenueLanguage      = 183
	WLANVenueName          = 184
	WLANReasonCode         = 185
	WLANPairwiseCipher     = 186
	WLANGroupCipher        = 187
	WLANKMSuite            = 188
	WLANGroupMgmtCipher    = 189
	WLANRFBand             = 190
)

/* =========================================================
 * RADIUS PORTS / CODES
 * ========================================================= */

const (
	RadiusAcctPort1   = 1813
	RadiusAcctPort2   = 1812
	RadiusCodeAcctReq = 4
)

/* =========================================================
 * VALID FIELD BITMASKS
 * ========================================================= */

type ValidField uint64

const (
	ValidAcctStatusType ValidField = 1 << iota
	ValidAcctSessionID
	ValidCallingStationID
	ValidFramedIPv4
	ValidFramedIPv6Prefix
	ValidEventTimestamp
	ValidAcctMultiSessionID
)

/* =========================================================
 * SESSION TYPES
 * ========================================================= */

type SessionType int

const (
	SessionStart SessionType = iota + 1
	SessionStop
	SessionUpdate
)

/* =========================================================
 * CORE CONSTANTS
 * ========================================================= */

const (
	IPv6PrefixMaxLen = 18
	IPv4Octets       = 4
	SessionIDMaxLen  = 64
	MaxExtraAVPs     = 256
	MaxAVPValue      = 256
	MaxQueueSize     = 4096
	MaxCoreCount     = 256
	MaxThreads       = 64
	MaxLine          = 512
)

/* =========================================================
 * ENUMS
 * ========================================================= */

type L4Protocol int

const (
	L4Unknown L4Protocol = iota
	L4TCP
	L4UDP
)

type PacketType int

const (
	PktUnknown PacketType = iota
	PktRadiusAuth
	PktRadiusAcct
)

/* =========================================================
 * HELPERS
 * ========================================================= */

type IPv4 [4]byte

func (ip IPv4) ToNetIP() net.IP {
	return net.IPv4(ip[0], ip[1], ip[2], ip[3])
}