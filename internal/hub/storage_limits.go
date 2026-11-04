package hub

import (
	"errors"
	"fmt"
	"github.com/shirou/gopsutil/v4/disk"
	"time"
)

var ErrStoragePressure = errors.New("storage pressure")

// SetStorageLimits is called before serving requests. SQLite enforces a page
// budget while still allowing deleted pages to be reused; WAL has normal
// checkpoint overhead. Filesystem free space is checked independently.
func (s *Store) SetStorageLimits(databaseMiB, minFreeMiB int) error {
	if databaseMiB < 64 || databaseMiB > 65536 || minFreeMiB < 16 || minFreeMiB > 1048576 {
		return errors.New("invalid storage limits")
	}
	var pageSize int
	if err := s.DB.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return err
	}
	pages := int64(databaseMiB) * 1024 * 1024 / int64(pageSize)
	var actual int64
	if err := s.DB.QueryRow(fmt.Sprintf("PRAGMA max_page_count=%d", pages)).Scan(&actual); err != nil {
		return err
	}
	if actual > pages {
		return errors.New("existing database exceeds requested page budget")
	}
	s.minFreeBytes = uint64(minFreeMiB) * 1024 * 1024
	return nil
}
func (s *Store) checkStorage() error {
	s.spaceMu.Lock()
	defer s.spaceMu.Unlock()
	if time.Since(s.spaceAt) < 10*time.Second {
		if s.spaceOK {
			return nil
		}
		return ErrStoragePressure
	}
	info, err := disk.Usage(s.Directory)
	s.spaceAt = time.Now()
	s.spaceOK = err == nil && info.Free >= s.minFreeBytes
	if !s.spaceOK {
		return ErrStoragePressure
	}
	return nil
}

func storagePressure(err error) bool {
	if errors.Is(err, ErrStoragePressure) {
		return true
	}
	var sqlite interface{ Code() int }
	return errors.As(err, &sqlite) && sqlite.Code()&255 == 13
}
