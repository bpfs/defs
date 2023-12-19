package erasure

import (
	"testing"

	"github.com/sirupsen/logrus"
)

func TestXxx(t *testing.T) {
	New("../合约 - 比特币.pdf", 3)
	// New("../合约 - 比特币.pdf", 0)
	// New("../合约 - 比特币.pdf", 5)
	// New("../lantern-installer.dmg")
}

func TestYyy(t *testing.T) {
	fi, err := ReadFile("../合约 - 比特币.pdf")
	if err != nil {
		logrus.Printf("%v", err.Error())
		return
	}

	logrus.Printf("Name\t%v", fi.name)
	logrus.Printf("Size\t%v", fi.size)
	// logrus.Printf("Mode\t%v", fi.Mode)
	logrus.Printf("ModTime\t%v", fi.modTime)
	// logrus.Printf("IsDir\t%v", fi.IsDir)
	// logrus.Printf("Sys\t%v", fi.Sys)
	logrus.Printf("Data\t%v", len(fi.data))
	logrus.Printf("Hash\t%s", fi.hash)

	for _, v := range fi.SubFileInfo {
		logrus.Printf("\n")
		logrus.Printf("==>\tsize\t%v", v.size)
		logrus.Printf("==>\tdata\t%v", len(v.data))
		logrus.Printf("==>\thash\t%v", v.hash)
		logrus.Printf("==>\tmod\t%v", v.mod)
	}

	// fi2, err := ReadFile("../123.dmg")
	// if err != nil {
	// 	logrus.Printf("%v", err.Error())
	// 	return
	// }
	// logrus.Printf("\n")
	// logrus.Printf("Name\t%v", fi2.name)
	// logrus.Printf("Size\t%v", fi2.size)
	// logrus.Printf("ModTime\t%v", fi2.modTime)
	// logrus.Printf("Data\t%v", len(fi2.data))
	// logrus.Printf("Hash\t%s", fi2.hash)

}

func TestAbcs(t *testing.T) {
	Abcs()
}
