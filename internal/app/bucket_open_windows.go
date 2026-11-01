package app

import "os"

// markDownloaded gives a copy the Mark of the Web, as a browser does for
// a download: Office then opens it in Protected View, and Windows warns
// before anything in it runs. A volume without streams, such as FAT,
// keeps no mark; the copy still opens.
func markDownloaded(file string) {
	_ = os.WriteFile(file+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\n"), 0o600)
}
