package dlna

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	tagObjectID       = regexp.MustCompile(`(?is)<ObjectID[^>]*>(.*?)</ObjectID>`)
	tagBrowseFlag     = regexp.MustCompile(`(?is)<BrowseFlag[^>]*>(.*?)</BrowseFlag>`)
	tagStartingIndex  = regexp.MustCompile(`(?is)<StartingIndex[^>]*>(.*?)</StartingIndex>`)
	tagRequestedCount = regexp.MustCompile(`(?is)<RequestedCount[^>]*>(.*?)</RequestedCount>`)
)

type ContentItem struct {
	ID          string
	ParentID    string
	Title       string
	Class       string
	MimeType    string
	Size        int64
	URL         string
	IsContainer bool
	ChildCount  int
}

func IsBrowseAction(soapAction string) bool {
	action := strings.ToLower(strings.TrimSpace(soapAction))
	return strings.Contains(action, "#browse")
}

func IsConnectionManagerAction(soapAction, actionName string) bool {
	action := strings.ToLower(strings.TrimSpace(soapAction))
	target := "#" + strings.ToLower(strings.TrimSpace(actionName))
	return strings.Contains(action, target)
}

func ParseBrowseRequest(body []byte) (objectID string, browseFlag string, start int, requested int, err error) {
	objectID = strings.TrimSpace(firstCapture(tagObjectID, string(body)))
	if objectID == "" {
		objectID = "0"
	}
	browseFlag = strings.TrimSpace(firstCapture(tagBrowseFlag, string(body)))
	if browseFlag == "" {
		browseFlag = "BrowseDirectChildren"
	}
	start = parseIntDefault(firstCapture(tagStartingIndex, string(body)), 0)
	requested = parseIntDefault(firstCapture(tagRequestedCount, string(body)), 0)
	return objectID, browseFlag, start, requested, nil
}

func BuildDeviceDescriptionXML(baseURL, friendlyName, uuid string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if friendlyName == "" {
		friendlyName = "Shelfy DLNA"
	}
	if uuid == "" {
		uuid = "shelfy"
	}
	udn := "uuid:" + strings.TrimPrefix(uuid, "uuid:")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0" xmlns:dlna="urn:schemas-dlna-org:device-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <URLBase>%s/</URLBase>
  <device>
    <deviceType>urn:schemas-upnp-org:device:MediaServer:1</deviceType>
    <friendlyName>%s</friendlyName>
    <manufacturer>Shelfy</manufacturer>
    <modelName>Shelfy DLNA Media Server</modelName>
    <modelNumber>1</modelNumber>
    <serialNumber>1</serialNumber>
    <dlna:X_DLNADOC>DMS-1.50</dlna:X_DLNADOC>
    <presentationURL>/</presentationURL>
    <UDN>%s</UDN>
    <serviceList>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ContentDirectory:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:ContentDirectory</serviceId>
        <controlURL>/dlna/control/content_directory</controlURL>
        <eventSubURL>/dlna/event/content_directory</eventSubURL>
        <SCPDURL>/dlna/scpd/content_directory.xml</SCPDURL>
      </service>
      <service>
        <serviceType>urn:schemas-upnp-org:service:ConnectionManager:1</serviceType>
        <serviceId>urn:upnp-org:serviceId:ConnectionManager</serviceId>
        <controlURL>/dlna/control/connection_manager</controlURL>
        <eventSubURL>/dlna/event/connection_manager</eventSubURL>
        <SCPDURL>/dlna/scpd/connection_manager.xml</SCPDURL>
      </service>
    </serviceList>
  </device>
</root>`, escapeXML(base), escapeXML(friendlyName), escapeXML(udn))
}

func BuildContentDirectorySCPD() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action>
      <name>Browse</name>
      <argumentList>
        <argument><name>ObjectID</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_ObjectID</relatedStateVariable></argument>
        <argument><name>BrowseFlag</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_BrowseFlag</relatedStateVariable></argument>
        <argument><name>Filter</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Filter</relatedStateVariable></argument>
        <argument><name>StartingIndex</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Index</relatedStateVariable></argument>
        <argument><name>RequestedCount</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument>
        <argument><name>SortCriteria</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_SortCriteria</relatedStateVariable></argument>
        <argument><name>Result</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Result</relatedStateVariable></argument>
        <argument><name>NumberReturned</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument>
        <argument><name>TotalMatches</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Count</relatedStateVariable></argument>
        <argument><name>UpdateID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_UpdateID</relatedStateVariable></argument>
      </argumentList>
    </action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ObjectID</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_BrowseFlag</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Filter</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Index</name><dataType>ui4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Count</name><dataType>ui4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_SortCriteria</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Result</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_UpdateID</name><dataType>ui4</dataType></stateVariable>
  </serviceStateTable>
</scpd>`
}

func BuildConnectionManagerSCPD() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<scpd xmlns="urn:schemas-upnp-org:service-1-0">
  <specVersion><major>1</major><minor>0</minor></specVersion>
  <actionList>
    <action>
      <name>GetProtocolInfo</name>
      <argumentList>
        <argument><name>Source</name><direction>out</direction><relatedStateVariable>SourceProtocolInfo</relatedStateVariable></argument>
        <argument><name>Sink</name><direction>out</direction><relatedStateVariable>SinkProtocolInfo</relatedStateVariable></argument>
      </argumentList>
    </action>
    <action>
      <name>GetCurrentConnectionIDs</name>
      <argumentList>
        <argument><name>ConnectionIDs</name><direction>out</direction><relatedStateVariable>CurrentConnectionIDs</relatedStateVariable></argument>
      </argumentList>
    </action>
    <action>
      <name>GetCurrentConnectionInfo</name>
      <argumentList>
        <argument><name>ConnectionID</name><direction>in</direction><relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable></argument>
        <argument><name>RcsID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_RcsID</relatedStateVariable></argument>
        <argument><name>AVTransportID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_AVTransportID</relatedStateVariable></argument>
        <argument><name>ProtocolInfo</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ProtocolInfo</relatedStateVariable></argument>
        <argument><name>PeerConnectionManager</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionManager</relatedStateVariable></argument>
        <argument><name>PeerConnectionID</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionID</relatedStateVariable></argument>
        <argument><name>Direction</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_Direction</relatedStateVariable></argument>
        <argument><name>Status</name><direction>out</direction><relatedStateVariable>A_ARG_TYPE_ConnectionStatus</relatedStateVariable></argument>
      </argumentList>
    </action>
  </actionList>
  <serviceStateTable>
    <stateVariable sendEvents="no"><name>SourceProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>SinkProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>CurrentConnectionIDs</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ConnectionID</name><dataType>i4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_RcsID</name><dataType>i4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_AVTransportID</name><dataType>i4</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ProtocolInfo</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ConnectionManager</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_Direction</name><dataType>string</dataType></stateVariable>
    <stateVariable sendEvents="no"><name>A_ARG_TYPE_ConnectionStatus</name><dataType>string</dataType></stateVariable>
  </serviceStateTable>
</scpd>`
}

func BuildGetProtocolInfoSOAP(protocols []string) string {
	filtered := make([]string, 0, len(protocols))
	for _, protocol := range protocols {
		protocol = strings.TrimSpace(protocol)
		if protocol == "" {
			continue
		}
		filtered = append(filtered, protocol)
	}
	source := strings.Join(filtered, ",")
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:GetProtocolInfoResponse xmlns:u="urn:schemas-upnp-org:service:ConnectionManager:1">
      <Source>%s</Source>
      <Sink></Sink>
    </u:GetProtocolInfoResponse>
  </s:Body>
</s:Envelope>`, escapeXML(source))
}

func BuildGetCurrentConnectionIDsSOAP() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:GetCurrentConnectionIDsResponse xmlns:u="urn:schemas-upnp-org:service:ConnectionManager:1">
      <ConnectionIDs></ConnectionIDs>
    </u:GetCurrentConnectionIDsResponse>
  </s:Body>
</s:Envelope>`
}

func BuildGetCurrentConnectionInfoSOAP() string {
	return `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:GetCurrentConnectionInfoResponse xmlns:u="urn:schemas-upnp-org:service:ConnectionManager:1">
      <RcsID>-1</RcsID>
      <AVTransportID>-1</AVTransportID>
      <ProtocolInfo></ProtocolInfo>
      <PeerConnectionManager></PeerConnectionManager>
      <PeerConnectionID>-1</PeerConnectionID>
      <Direction>Output</Direction>
      <Status>OK</Status>
    </u:GetCurrentConnectionInfoResponse>
  </s:Body>
</s:Envelope>`
}

func BuildBrowseSOAP(items []ContentItem, start, requested int) string {
	total := len(items)
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := total
	if requested > 0 && start+requested < end {
		end = start + requested
	}
	sliced := items[start:end]
	result := buildDIDLLite(sliced)
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:BrowseResponse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1">
      <Result>%s</Result>
      <NumberReturned>%d</NumberReturned>
      <TotalMatches>%d</TotalMatches>
      <UpdateID>1</UpdateID>
    </u:BrowseResponse>
  </s:Body>
</s:Envelope>`, escapeXML(result), len(sliced), total)
}

func BuildSOAPFault(code, message string) string {
	if code == "" {
		code = "401"
	}
	if message == "" {
		message = "Invalid Action"
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">
  <s:Body>
    <s:Fault>
      <faultcode>s:Client</faultcode>
      <faultstring>UPnPError</faultstring>
      <detail>
        <UPnPError xmlns="urn:schemas-upnp-org:control-1-0">
          <errorCode>%s</errorCode>
          <errorDescription>%s</errorDescription>
        </UPnPError>
      </detail>
    </s:Fault>
  </s:Body>
</s:Envelope>`, escapeXML(code), escapeXML(message))
}

func buildDIDLLite(items []ContentItem) string {
	var b strings.Builder
	b.WriteString(`<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">`)
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		parentID := strings.TrimSpace(item.ParentID)
		if parentID == "" {
			parentID = "0"
		}
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = id
		}
		if item.IsContainer {
			class := strings.TrimSpace(item.Class)
			if class == "" {
				class = "object.container.storageFolder"
			}
			b.WriteString(`<container id="` + escapeXML(id) + `" parentID="` + escapeXML(parentID) + `" restricted="1" searchable="0"`)
			if item.ChildCount >= 0 {
				b.WriteString(` childCount="` + strconv.Itoa(item.ChildCount) + `"`)
			}
			b.WriteString(`>`)
			b.WriteString(`<dc:title>` + escapeXML(title) + `</dc:title>`)
			b.WriteString(`<upnp:class>` + escapeXML(class) + `</upnp:class>`)
			b.WriteString(`</container>`)
			continue
		}
		class := strings.TrimSpace(item.Class)
		if class == "" {
			class = "object.item"
		}
		protocol := "http-get:*:*:*"
		if strings.TrimSpace(item.MimeType) != "" {
			protocol = "http-get:*:" + strings.TrimSpace(item.MimeType) + ":*"
		}
		b.WriteString(`<item id="` + escapeXML(id) + `" parentID="` + escapeXML(parentID) + `" restricted="1">`)
		b.WriteString(`<dc:title>` + escapeXML(title) + `</dc:title>`)
		b.WriteString(`<upnp:class>` + escapeXML(class) + `</upnp:class>`)
		if item.Size > 0 {
			b.WriteString(`<res protocolInfo="` + escapeXML(protocol) + `" size="` + strconv.FormatInt(item.Size, 10) + `">` + escapeXML(item.URL) + `</res>`)
		} else {
			b.WriteString(`<res protocolInfo="` + escapeXML(protocol) + `">` + escapeXML(item.URL) + `</res>`)
		}
		b.WriteString(`</item>`)
	}
	b.WriteString(`</DIDL-Lite>`)
	return b.String()
}

func escapeXML(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstCapture(re *regexp.Regexp, body string) string {
	match := re.FindStringSubmatch(body)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func parseIntDefault(raw string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}
