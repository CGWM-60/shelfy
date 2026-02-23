package dlna

import (
	"strings"
	"testing"
)

func TestBuildDeviceDescriptionXML(t *testing.T) {
	xml := BuildDeviceDescriptionXML("http://127.0.0.1:8080", "Shelfy", "abc-123")
	if !strings.Contains(xml, "MediaServer:1") {
		t.Fatalf("missing mediaserver type")
	}
	if !strings.Contains(xml, "<friendlyName>Shelfy</friendlyName>") {
		t.Fatalf("missing friendly name")
	}
	if !strings.Contains(xml, "<UDN>uuid:abc-123</UDN>") {
		t.Fatalf("missing udn")
	}
	if !strings.Contains(xml, "ConnectionManager:1") {
		t.Fatalf("missing connection manager service")
	}
}

func TestParseBrowseRequest(t *testing.T) {
	body := []byte(`<s:Envelope><s:Body><u:Browse xmlns:u="urn:schemas-upnp-org:service:ContentDirectory:1"><ObjectID>0</ObjectID><BrowseFlag>BrowseMetadata</BrowseFlag><StartingIndex>2</StartingIndex><RequestedCount>5</RequestedCount></u:Browse></s:Body></s:Envelope>`)
	objectID, browseFlag, start, count, err := ParseBrowseRequest(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if objectID != "0" || browseFlag != "BrowseMetadata" || start != 2 || count != 5 {
		t.Fatalf("unexpected parse result: objectID=%q browseFlag=%q start=%d count=%d", objectID, browseFlag, start, count)
	}
}

func TestBuildBrowseSOAPPagination(t *testing.T) {
	items := []ContentItem{
		{ID: "m1", Title: "A", MimeType: "video/mp4", Class: "object.item.videoItem", URL: "http://local/dlna/media/m1"},
		{ID: "m2", Title: "B", MimeType: "video/mp4", Class: "object.item.videoItem", URL: "http://local/dlna/media/m2"},
		{ID: "m3", Title: "C", MimeType: "video/mp4", Class: "object.item.videoItem", URL: "http://local/dlna/media/m3"},
	}
	soap := BuildBrowseSOAP(items, 1, 1)
	if !strings.Contains(soap, "<NumberReturned>1</NumberReturned>") {
		t.Fatalf("expected one returned item")
	}
	if !strings.Contains(soap, "<TotalMatches>3</TotalMatches>") {
		t.Fatalf("expected total matches")
	}
	if !strings.Contains(soap, "m2") || strings.Contains(soap, "m1") {
		t.Fatalf("expected only m2 in page: %s", soap)
	}
}

func TestIsBrowseAction(t *testing.T) {
	if !IsBrowseAction(`"urn:schemas-upnp-org:service:ContentDirectory:1#Browse"`) {
		t.Fatalf("expected browse action")
	}
	if IsBrowseAction(`"urn:schemas-upnp-org:service:ContentDirectory:1#Search"`) {
		t.Fatalf("search is not browse")
	}
}

func TestConnectionManagerPayloads(t *testing.T) {
	scpd := BuildConnectionManagerSCPD()
	if !strings.Contains(scpd, "GetProtocolInfo") {
		t.Fatalf("missing GetProtocolInfo action")
	}
	soap := BuildGetProtocolInfoSOAP([]string{"http-get:*:video/mp4:*"})
	if !strings.Contains(soap, "GetProtocolInfoResponse") || !strings.Contains(soap, "video/mp4") {
		t.Fatalf("unexpected protocol info response: %s", soap)
	}
	if !IsConnectionManagerAction(`"urn:schemas-upnp-org:service:ConnectionManager:1#GetProtocolInfo"`, "GetProtocolInfo") {
		t.Fatalf("expected connection manager action match")
	}
	if IsConnectionManagerAction(`"urn:schemas-upnp-org:service:ConnectionManager:1#Browse"`, "GetProtocolInfo") {
		t.Fatalf("unexpected connection manager action match")
	}
}

func TestBuildBrowseSOAPContainer(t *testing.T) {
	soap := BuildBrowseSOAP([]ContentItem{{ID: "dir:0:Movies", ParentID: "0", Title: "Movies", IsContainer: true, ChildCount: 2}}, 0, 0)
	if !strings.Contains(soap, "&lt;container id=&quot;dir:0:Movies&quot;") {
		t.Fatalf("expected container item: %s", soap)
	}
	if !strings.Contains(soap, "childCount=&quot;2&quot;") {
		t.Fatalf("expected childCount attribute: %s", soap)
	}
}
