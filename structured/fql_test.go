package bpfsfql

import (
	"testing"
)

func TestExecCreateFile(t *testing.T) {

	// query := `CREATE FILE filename OWNED_BY=did CUSTOM_NAME=custom_name METADATA="key1:value1,key2:value2"`
	query := `CREATE FILE "filename" OWNED_BY="did" CUSTOM_NAME="custom_name" METADATA="key1:value1,key2:value2"`
	_, err := Exec(query)
	if err != nil {
		t.Fatalf("Exec failed: %s", err)
	}

}
