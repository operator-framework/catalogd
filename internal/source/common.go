package source

import (
	"fmt"
)

func generateMessage(catalogName string) string {
	return fmt.Sprintf("Successfully unpacked the %s Bundle", catalogName)
}
