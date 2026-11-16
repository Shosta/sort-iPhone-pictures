package tagger

import (
	"bytes"
	"encoding/xml"
	"os"
)

// XMP renders an XMP packet listing tags as dc:subject keywords, readable by
// Lightroom, digiKam, Photos and most DAMs when stored next to the photo.
func XMP(tags []string) []byte {
	var items bytes.Buffer
	for _, tag := range tags {
		items.WriteString("     <rdf:li>")
		xml.EscapeText(&items, []byte(tag))
		items.WriteString("</rdf:li>\n")
	}
	var out bytes.Buffer
	out.WriteString(`<?xpacket begin="` + "\ufeff" + `" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">
   <dc:subject>
    <rdf:Bag>
`)
	out.Write(items.Bytes())
	out.WriteString(`    </rdf:Bag>
   </dc:subject>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>
`)
	return out.Bytes()
}

// WriteXMP writes the tags to path without overwriting an existing file.
func WriteXMP(path string, tags []string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = file.Write(XMP(tags))
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}
