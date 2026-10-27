package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/itsgiahung/502Drive/internal/filter"
	"github.com/spf13/cobra"
)

// flagSet is the part of pflag.FlagSet the filter flags use (no new import).
type flagSet interface {
	StringVar(p *string, name, value, usage string)
	StringArrayVar(p *[]string, name string, value []string, usage string)
}

// filterFlags are the selective-sync flags shared by inspect, copy and
// sync filter set.
type filterFlags struct {
	include, exclude []string
	ext              string
	minSize, maxSize string
	file             string
	keepEmpty        bool
}

func (f *filterFlags) bind(fs flagSet) {
	fs.StringArrayVar(&f.include, "include", nil, "copy only matching entries: glob (name, or path with /), ext:.pdf or class:doc (repeatable)")
	fs.StringArrayVar(&f.exclude, "exclude", nil, "skip matching entries; wins over --include (glob, ext:, class:; repeatable)")
	fs.StringVar(&f.ext, "ext", "", "comma-separated extensions to include, e.g. pdf,docx")
	fs.StringVar(&f.minSize, "min-size", "", "skip files smaller than this (bytes or KB, MB, GB, TB)")
	fs.StringVar(&f.maxSize, "max-size", "", "skip files larger than this (bytes or KB, MB, GB, TB)")
	fs.StringVar(&f.file, "filter-file", "", "read rules from a file: one `include|exclude|ext|min-size|max-size|modified-after|keep-empty-folders` per line")
}

// spec combines the file (if any) with the flags; flags add to file rules.
func (f *filterFlags) spec() (filter.Spec, error) {
	var s filter.Spec
	if f.file != "" {
		fh, err := os.Open(f.file)
		if err != nil {
			return s, usage("--filter-file: %v", err)
		}
		defer fh.Close()
		if s, err = parseFilterFile(fh); err != nil {
			return s, usage("--filter-file %s: %v", f.file, err)
		}
	}
	s.Include = append(s.Include, f.include...)
	s.Exclude = append(s.Exclude, f.exclude...)
	for _, e := range strings.Split(f.ext, ",") {
		if e = strings.TrimSpace(e); e != "" {
			s.Include = append(s.Include, "ext:"+e)
		}
	}
	if f.minSize != "" {
		n, err := filter.ParseSize(f.minSize)
		if err != nil {
			return s, usage("--min-size: %v", err)
		}
		s.MinSize = n
	}
	if f.maxSize != "" {
		n, err := filter.ParseSize(f.maxSize)
		if err != nil {
			return s, usage("--max-size: %v", err)
		}
		s.MaxSize = n
	}
	if f.keepEmpty {
		s.KeepEmptyFolders = true
	}
	if err := s.Validate(); err != nil {
		return s, usage("%v", err)
	}
	return s, nil
}

// parseFilterFile reads the line format documented for --filter-file. Blank
// lines and lines starting with # are ignored.
func parseFilterFile(r io.Reader) (filter.Spec, error) {
	var s filter.Spec
	sc := bufio.NewScanner(io.LimitReader(r, 256<<10))
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, _ := strings.Cut(line, " ")
		val = strings.TrimSpace(val)
		var err error
		switch key {
		case "include":
			s.Include = append(s.Include, val)
		case "exclude":
			s.Exclude = append(s.Exclude, val)
		case "ext":
			s.Include = append(s.Include, "ext:"+strings.TrimPrefix(val, "."))
		case "min-size":
			s.MinSize, err = filter.ParseSize(val)
		case "max-size":
			s.MaxSize, err = filter.ParseSize(val)
		case "modified-after":
			s.ModifiedAfter = val
		case "keep-empty-folders":
			s.KeepEmptyFolders = true
		default:
			err = fmt.Errorf("unknown rule %q", key)
		}
		if err != nil {
			return s, fmt.Errorf("line %d: %v", n, err)
		}
	}
	return s, sc.Err()
}

// syncFilterCmd is `sync filter show|set COLLECTION`. Setting a rule changes
// what later runs copy; nothing already at the destination is removed.
func (a *App) syncFilterCmd() *cobra.Command {
	fc := &cobra.Command{Use: "filter", Short: "Show or change which entries later sync runs copy"}
	fc.AddCommand(a.syncFilterShowCmd(), a.syncFilterSetCmd())
	return fc
}

func (a *App) syncFilterShowCmd() *cobra.Command {
	return &cobra.Command{Use: "show COLLECTION", Short: "Show the selective-sync rule of a copied source", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := a.call(cmd.Context(), "GET", "/v1/collections/"+url.PathEscape(args[0])+"/filter", nil, nil, "")
			if err != nil {
				return err
			}
			var v filterView
			if err := r.Decode(&v); err != nil {
				return err
			}
			a.emit(r, v.CollectionID, func() { a.printFilter(v) })
			return nil
		}}
}

func (a *App) syncFilterSetCmd() *cobra.Command {
	var ff filterFlags
	var clear bool
	c := &cobra.Command{Use: "set COLLECTION", Short: "Replace the selective-sync rule (flags, --filter-file, or --clear)", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec := filter.Spec{}
			if !clear {
				s, err := ff.spec()
				if err != nil {
					return err
				}
				spec = s
			}
			cur, err := a.call(cmd.Context(), "GET", "/v1/collections/"+url.PathEscape(args[0])+"/filter", nil, nil, "")
			if err != nil {
				return err
			}
			var v filterView
			if err := cur.Decode(&v); err != nil {
				return err
			}
			body := map[string]any{"filter": spec, "expected_revision": v.Revision}
			r, err := a.call(cmd.Context(), "PUT", "/v1/collections/"+url.PathEscape(args[0])+"/filter", nil, body, "")
			if err != nil {
				return err
			}
			var out filterView
			if err := r.Decode(&out); err != nil {
				return err
			}
			a.emit(r, out.CollectionID, func() {
				fmt.Fprintln(a.out(), "filter saved; it applies to the next run")
				a.printFilter(out)
			})
			return nil
		}}
	ff.bind(c.Flags())
	c.Flags().BoolVar(&ff.keepEmpty, "keep-empty-folders", false, "still create folders that end up with nothing kept")
	c.Flags().BoolVar(&clear, "clear", false, "remove every rule (copy everything again on later runs)")
	return c
}

type filterView struct {
	CollectionID string      `json:"collection_id"`
	Revision     int         `json:"revision"`
	Filter       filter.Spec `json:"filter"`
}

func (a *App) printFilter(v filterView) {
	f := v.Filter
	if !f.Active() && !f.KeepEmptyFolders {
		fmt.Fprintln(a.out(), "no filter: every entry is copied")
		return
	}
	printKV(a.out(), a.style, []kv{
		{"include", joinOrDash(f.Include)}, {"exclude", joinOrDash(f.Exclude)},
		{"min size", sizeOrDash(f.MinSize)}, {"max size", sizeOrDash(f.MaxSize)},
		{"modified after", dashIf(f.ModifiedAfter)}, {"keep empty folders", fmt.Sprint(f.KeepEmptyFolders)},
	})
}

func joinOrDash(xs []string) string {
	if len(xs) == 0 {
		return "-"
	}
	return strings.Join(xs, ", ")
}

func dashIf(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func sizeOrDash(n int64) string {
	if n == 0 {
		return "-"
	}
	return strconv.FormatInt(n, 10) + " bytes"
}
