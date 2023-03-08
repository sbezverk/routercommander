package patterns

import "regexp"

// ShowL3Interface identifies the beginning of usable information in show ip interface brief command
var ShowL3Interface = regexp.MustCompile("Interface                      IP-Address      Status          Protocol Vrf-Name")

// Every command is followed by exit which helps to identify the end of output
var Exit = regexp.MustCompile("#exit")

// Identify active interfaces

var ActiveL3Interface = regexp.MustCompile(`Up\s+Up`)

// Substring separated by space

var SubStringSeparator = regexp.MustCompile(`\s+`)

var SubStringSeparatorColon = regexp.MustCompile(`:`)

var SubStringSeparatorArrow = regexp.MustCompile(`->`)

var SubStringSeparatorComma = regexp.MustCompile(`,`)

var SubStringSeparatorVerticalBar = regexp.MustCompile(`\|`)

var InterfaceName = regexp.MustCompile(`[a-zA-Z\-]+`)

var InterfaceNumber = regexp.MustCompile(`[0-9]+`)

var InterfaceLocation = regexp.MustCompile(`/`)

var LinecardType = regexp.MustCompile(`Asic Type\s+:`)

var Location = regexp.MustCompile(`\d+\/(RP)?\d+\/CPU0\s+[a-zA-Z0-9\-\(\)]+\s+IOS XR RUN\s+`)

var ActiveMember = regexp.MustCompile(`\w+[0-9](/[0-9]+)*\s+\w+\s+Active\s+.*`)

var Prompt = regexp.MustCompile(`(?m)RP\/\d\/(RP)?\d\/CPU[0-9]:[0-9A-Za-z-\.\_]+(\([0-9A-Za-z-\.\_]+\))?#(\n|$)`)

// Regular expressions used for parsing  show route  output
var IPv4 = regexp.MustCompile(`(?m)(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\,\s+from`)
var IPv6 = regexp.MustCompile(`(?m)(?:[A-F0-9]{1,4}:){7}[A-F0-9]{1,4}\,`)
var Metric = regexp.MustCompile(`Route metric is`)
var Label = regexp.MustCompile(`^Label:`)
var PathID = regexp.MustCompile(`^Path\s+id:`)
var NHID = regexp.MustCompile(`NHID:`)
var NHIDeid = regexp.MustCompile(`NHID\s+eid:`)
var MPLSeid = regexp.MustCompile(`MPLS\s+eid:`)
var BackupPathID = regexp.MustCompile(`Backup\s+path\s+id:`)

var RouteEntry = regexp.MustCompile(`^Routing\s+entry\s+for\s+`)
var KnownVia = regexp.MustCompile(`^Known via`)
var LocalLabel = regexp.MustCompile(`^Local Label:`)
var RouteVersion = regexp.MustCompile(`(?m)\s*Route version is`)
var RDB = regexp.MustCompile(`(?m)\s*Routing Descriptor Blocks$`)

// Regular expressions used for parsing show cef
var IPv4CEF = regexp.MustCompile(`(?m)(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/[0-9]+\,\s+version`)
var LabelCEF = regexp.MustCompile(`^[0-9]+\s+([0-9]+|Pop|Exp-Null-v4)\s+`)
var LocAdjCEF = regexp.MustCompile(`(?m)local\s+adjacency\s+`)
var ViaCEF = regexp.MustCompile(`(?m)via\s+(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/[0-9]+\,`)
var NHIDCEF = regexp.MustCompile(`path-idx\s+\b(0x[0-9a-fA-F]+|[0-9]+)\b\s+(bkup-idx\s+\b(0x[0-9a-fA-F]+|[0-9]+)\b\s+)?NHID\s+\b(0x[0-9a-fA-F]+|[0-9]+)\b`)
var NextHopCEF = regexp.MustCompile(`(?m)next\s+hop\s+(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)`)
var LocalLabelCEF = regexp.MustCompile(`\s*local\s+label\s+[\d]+\s+labels\s+imposed\s+`)

var LeafCEF = regexp.MustCompile(`LEAF:`)
var LWLDICEF = regexp.MustCompile(`\sLWLDI:`)
var RSHLDICEF = regexp.MustCompile(`\sRSHLDI:`)
var SHLDICEF = regexp.MustCompile(`\sSHLDI:`)
var TXNHInfoCEF = regexp.MustCompile(`TX-NHINFO:`)

// Complete route patterns

var ShowRouteEntry = regexp.MustCompile(`[B|O|i]\s+(?:\w[1-2]\s+)?(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/[0-9]+\s+`)

// show cef X.X.X.X/y detail patterns
var ViaCEF_PI = regexp.MustCompile(`(?m)^\s*via\s+(?:\w[1-2]\s+)?(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)/[0-9]+,`)
