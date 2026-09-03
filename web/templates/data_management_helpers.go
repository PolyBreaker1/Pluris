package templates

import (
	"fmt"
	"strings"

	"github.com/pluris/pluris/db"
)

func retentionKindLabel(kind string) string {
	return strings.NewReplacer("_", " ").Replace(strings.Title(kind)) //nolint:staticcheck
}

func retentionDaysValue(setting db.RetentionSetting) string {
	if setting.PurgeAfterDays == nil {
		return ""
	}
	return fmt.Sprint(setting.PurgeAfterDays)
}
