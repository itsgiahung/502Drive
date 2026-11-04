package state

import (
	"context"
	"testing"
)

func cleanupTotalsFixture(t testing.TB, n int) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, q := range []string{
		`INSERT INTO plans VALUES('plan','owner','{}',NULL)`,
		`INSERT INTO collections(id,principal,plan_id,body) VALUES('col','owner','plan','{}')`,
		`INSERT INTO jobs VALUES('job','owner','col','succeeded','{}')`,
	} {
		if _, err := s.db.ExecContext(context.Background(), q); err != nil {
			t.Fatal(err)
		}
	}
	_, err = s.db.ExecContext(context.Background(), `WITH RECURSIVE nums(n) AS (
		VALUES(1) UNION ALL SELECT n+1 FROM nums WHERE n<?)
		INSERT INTO items(collection_id,source_id,job_id,parent_source_id,kind,name,state,dest_id,size,list_state)
		SELECT 'col',printf('file%05d',n),'job','root','file','synthetic.bin','done',printf('copy%d',n),10,'done' FROM nums`, n)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCleanupTotalsUsesBoundedMemory(t *testing.T) {
	s := cleanupTotalsFixture(t, 1000)
	for _, q := range []string{
		`UPDATE items SET state='skipped' WHERE source_id='file00001'`,
		`UPDATE items SET dest_id='' WHERE source_id='file00002'`,
		`UPDATE items SET size=NULL WHERE source_id='file00003'`,
	} {
		if _, err := s.db.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	allocs := testing.AllocsPerRun(1, func() {
		count, bytes, err := s.CleanupTotals(t.Context(), "col")
		if err != nil || count != 998 || bytes != 9970 {
			t.Fatalf("totals=%d/%d error=%v", count, bytes, err)
		}
	})
	if allocs > 500 {
		t.Fatalf("cleanup loaded per-item metadata: %.0f allocations for 1000 rows", allocs)
	}
	if count, bytes, err := s.CleanupTotals(t.Context(), "missing"); err != nil || count != 0 || bytes != 0 {
		t.Fatalf("empty totals=%d/%d error=%v", count, bytes, err)
	}
}

func BenchmarkCleanupTotals(b *testing.B) {
	s := cleanupTotalsFixture(b, 10000)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := s.CleanupTotals(context.Background(), "col"); err != nil {
			b.Fatal(err)
		}
	}
}
