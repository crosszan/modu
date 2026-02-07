package format

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crosszan/modu/pkg/mmq"
)

// Format 输出格式类型
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
	FormatCSV  Format = "csv"
	FormatMD   Format = "md"
	FormatXML  Format = "xml"
)

// OutputDocumentList 输出文档列表
func OutputDocumentList(docs []mmq.DocumentListEntry, format Format) error {
	switch format {
	case FormatJSON:
		return outputJSON(docs)
	case FormatCSV:
		return outputDocListCSV(docs)
	case FormatMD:
		return outputDocListMarkdown(docs)
	case FormatXML:
		return outputXML(docs)
	default:
		return outputDocListText(docs)
	}
}

// OutputDocumentDetail 输出文档详情
func OutputDocumentDetail(doc *mmq.DocumentDetail, format Format, full bool, lineNumbers bool) error {
	switch format {
	case FormatJSON:
		return outputJSON(doc)
	case FormatCSV:
		// CSV不适合单个文档，使用文本
		return outputDocDetailText(doc, full, lineNumbers)
	case FormatMD:
		return outputDocDetailMarkdown(doc, full, lineNumbers)
	case FormatXML:
		return outputXML(doc)
	default:
		return outputDocDetailText(doc, full, lineNumbers)
	}
}

// OutputDocumentDetails 输出多个文档详情
func OutputDocumentDetails(docs []mmq.DocumentDetail, format Format, full bool, lineNumbers bool) error {
	switch format {
	case FormatJSON:
		return outputJSON(docs)
	case FormatCSV:
		return outputDocDetailsCSV(docs, full)
	case FormatMD:
		return outputDocDetailsMarkdown(docs, full, lineNumbers)
	case FormatXML:
		return outputXML(docs)
	default:
		return outputDocDetailsText(docs, full, lineNumbers)
	}
}

// OutputSearchResults 输出搜索结果
func OutputSearchResults(results []mmq.SearchResult, format Format, full bool) error {
	switch format {
	case FormatJSON:
		return outputJSON(results)
	case FormatCSV:
		return outputSearchCSV(results)
	case FormatMD:
		return outputSearchMarkdown(results, full)
	case FormatXML:
		return outputXML(results)
	default:
		return outputSearchText(results, full)
	}
}

// OutputCollections 输出集合列表
func OutputCollections(collections []mmq.Collection, format Format) error {
	switch format {
	case FormatJSON:
		return outputJSON(collections)
	case FormatCSV:
		return outputCollectionsCSV(collections)
	case FormatMD:
		return outputCollectionsMarkdown(collections)
	case FormatXML:
		return outputXML(collections)
	default:
		return outputCollectionsText(collections)
	}
}

// OutputContexts 输出上下文列表
func OutputContexts(contexts []mmq.ContextEntry, format Format) error {
	switch format {
	case FormatJSON:
		return outputJSON(contexts)
	case FormatCSV:
		return outputContextsCSV(contexts)
	case FormatMD:
		return outputContextsMarkdown(contexts)
	case FormatXML:
		return outputXML(contexts)
	default:
		return outputContextsText(contexts)
	}
}

// OutputStatus 输出状态信息
func OutputStatus(status mmq.Status, format Format) error {
	switch format {
	case FormatJSON:
		return outputJSON(status)
	case FormatMD:
		return outputStatusMarkdown(status)
	case FormatXML:
		return outputXML(status)
	default:
		return outputStatusText(status)
	}
}

// --- JSON 输出 ---
func outputJSON(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

// --- XML 输出 ---
func outputXML(v interface{}) error {
	encoder := xml.NewEncoder(os.Stdout)
	encoder.Indent("", "  ")
	if err := encoder.Encode(v); err != nil {
		return err
	}
	fmt.Println()
	return nil
}

// --- 文档列表输出 ---

func outputDocListText(docs []mmq.DocumentListEntry) error {
	for _, doc := range docs {
		fmt.Printf("%s %s/%s\n", doc.DocID, doc.Collection, doc.Path)
		fmt.Printf("  Title: %s\n", doc.Title)
		fmt.Printf("  Modified: %s\n", doc.ModifiedAt.Format(time.RFC3339))
		fmt.Println()
	}
	return nil
}

func outputDocListCSV(docs []mmq.DocumentListEntry) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Header
	w.Write([]string{"DocID", "Collection", "Path", "Title", "Modified"})

	// Rows
	for _, doc := range docs {
		w.Write([]string{
			doc.DocID,
			doc.Collection,
			doc.Path,
			doc.Title,
			doc.ModifiedAt.Format(time.RFC3339),
		})
	}

	return nil
}

func outputDocListMarkdown(docs []mmq.DocumentListEntry) error {
	fmt.Println("| DocID | Collection | Path | Title | Modified |")
	fmt.Println("|-------|------------|------|-------|----------|")

	for _, doc := range docs {
		fmt.Printf("| %s | %s | %s | %s | %s |\n",
			doc.DocID,
			doc.Collection,
			doc.Path,
			doc.Title,
			doc.ModifiedAt.Format("2006-01-02"),
		)
	}

	return nil
}

// --- 文档详情输出 ---

func outputDocDetailText(doc *mmq.DocumentDetail, full bool, lineNumbers bool) error {
	fmt.Printf("DocID: %s\n", doc.DocID)
	fmt.Printf("Collection: %s\n", doc.Collection)
	fmt.Printf("Path: %s\n", doc.Path)
	fmt.Printf("Title: %s\n", doc.Title)
	fmt.Printf("Modified: %s\n", doc.ModifiedAt.Format(time.RFC3339))
	fmt.Println()

	content := doc.Content
	if !full && len(content) > 500 {
		content = content[:500] + "\n..."
	}

	if lineNumbers {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			fmt.Printf("%4d | %s\n", i+1, line)
		}
	} else {
		fmt.Println(content)
	}

	return nil
}

func outputDocDetailMarkdown(doc *mmq.DocumentDetail, full bool, lineNumbers bool) error {
	fmt.Printf("# %s\n\n", doc.Title)
	fmt.Printf("**DocID:** %s  \n", doc.DocID)
	fmt.Printf("**Path:** %s/%s  \n", doc.Collection, doc.Path)
	fmt.Printf("**Modified:** %s\n\n", doc.ModifiedAt.Format("2006-01-02 15:04:05"))
	fmt.Println("---\n")

	content := doc.Content
	if !full && len(content) > 500 {
		content = content[:500] + "\n..."
	}

	fmt.Println(content)
	return nil
}

func outputDocDetailsText(docs []mmq.DocumentDetail, full bool, lineNumbers bool) error {
	for i, doc := range docs {
		if i > 0 {
			fmt.Println("\n" + strings.Repeat("=", 80) + "\n")
		}
		if err := outputDocDetailText(&doc, full, lineNumbers); err != nil {
			return err
		}
	}
	return nil
}

func outputDocDetailsMarkdown(docs []mmq.DocumentDetail, full bool, lineNumbers bool) error {
	for i, doc := range docs {
		if i > 0 {
			fmt.Println("\n---\n")
		}
		if err := outputDocDetailMarkdown(&doc, full, lineNumbers); err != nil {
			return err
		}
	}
	return nil
}

func outputDocDetailsCSV(docs []mmq.DocumentDetail, full bool) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	// Header
	w.Write([]string{"DocID", "Collection", "Path", "Title", "Content"})

	// Rows
	for _, doc := range docs {
		content := doc.Content
		if !full && len(content) > 500 {
			content = content[:500] + "..."
		}

		w.Write([]string{
			doc.DocID,
			doc.Collection,
			doc.Path,
			doc.Title,
			content,
		})
	}

	return nil
}

// --- 搜索结果输出 ---

func outputSearchText(results []mmq.SearchResult, full bool) error {
	for i, r := range results {
		fmt.Printf("[%d] Score: %.4f | %s/%s\n", i+1, r.Score, r.Collection, r.Path)
		fmt.Printf("    Title: %s\n", r.Title)

		if full {
			fmt.Printf("    Content:\n")
			lines := strings.Split(r.Content, "\n")
			for _, line := range lines {
				fmt.Printf("    %s\n", line)
			}
		} else if r.Snippet != "" {
			fmt.Printf("    Snippet: %s\n", r.Snippet)
		}

		fmt.Println()
	}
	return nil
}

func outputSearchMarkdown(results []mmq.SearchResult, full bool) error {
	fmt.Println("# Search Results\n")

	for i, r := range results {
		fmt.Printf("## %d. %s (%.4f)\n\n", i+1, r.Title, r.Score)
		fmt.Printf("**Path:** %s/%s  \n", r.Collection, r.Path)
		fmt.Printf("**Source:** %s\n\n", r.Source)

		if full {
			fmt.Println("```")
			fmt.Println(r.Content)
			fmt.Println("```\n")
		} else if r.Snippet != "" {
			fmt.Printf("> %s\n\n", r.Snippet)
		}
	}

	return nil
}

func outputSearchCSV(results []mmq.SearchResult) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	w.Write([]string{"Rank", "Score", "Collection", "Path", "Title", "Source", "Snippet"})

	for i, r := range results {
		w.Write([]string{
			fmt.Sprintf("%d", i+1),
			fmt.Sprintf("%.4f", r.Score),
			r.Collection,
			r.Path,
			r.Title,
			r.Source,
			r.Snippet,
		})
	}

	return nil
}

// --- 集合输出 ---

func outputCollectionsText(collections []mmq.Collection) error {
	for _, c := range collections {
		fmt.Printf("Collection: %s\n", c.Name)
		fmt.Printf("  Path: %s\n", c.Path)
		fmt.Printf("  Mask: %s\n", c.Mask)
		fmt.Printf("  Documents: %d\n", c.DocCount)
		fmt.Printf("  Updated: %s\n", c.UpdatedAt.Format(time.RFC3339))
		fmt.Println()
	}
	return nil
}

func outputCollectionsCSV(collections []mmq.Collection) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	w.Write([]string{"Name", "Path", "Mask", "DocCount", "Updated"})

	for _, c := range collections {
		w.Write([]string{
			c.Name,
			c.Path,
			c.Mask,
			fmt.Sprintf("%d", c.DocCount),
			c.UpdatedAt.Format(time.RFC3339),
		})
	}

	return nil
}

func outputCollectionsMarkdown(collections []mmq.Collection) error {
	fmt.Println("| Name | Path | Mask | Docs | Updated |")
	fmt.Println("|------|------|------|------|---------|")

	for _, c := range collections {
		fmt.Printf("| %s | %s | %s | %d | %s |\n",
			c.Name,
			c.Path,
			c.Mask,
			c.DocCount,
			c.UpdatedAt.Format("2006-01-02"),
		)
	}

	return nil
}

// --- 上下文输出 ---

func outputContextsText(contexts []mmq.ContextEntry) error {
	for _, ctx := range contexts {
		fmt.Printf("Path: %s\n", ctx.Path)
		fmt.Printf("  Content: %s\n", ctx.Content)
		fmt.Printf("  Updated: %s\n", ctx.UpdatedAt.Format(time.RFC3339))
		fmt.Println()
	}
	return nil
}

func outputContextsCSV(contexts []mmq.ContextEntry) error {
	w := csv.NewWriter(os.Stdout)
	defer w.Flush()

	w.Write([]string{"Path", "Content", "Updated"})

	for _, ctx := range contexts {
		w.Write([]string{
			ctx.Path,
			ctx.Content,
			ctx.UpdatedAt.Format(time.RFC3339),
		})
	}

	return nil
}

func outputContextsMarkdown(contexts []mmq.ContextEntry) error {
	fmt.Println("| Path | Content | Updated |")
	fmt.Println("|------|---------|---------|")

	for _, ctx := range contexts {
		fmt.Printf("| %s | %s | %s |\n",
			ctx.Path,
			ctx.Content,
			ctx.UpdatedAt.Format("2006-01-02"),
		)
	}

	return nil
}

// --- 状态输出 ---

func outputStatusText(status mmq.Status) error {
	fmt.Printf("Database: %s\n", status.DBPath)
	fmt.Printf("Cache Dir: %s\n", status.CacheDir)
	fmt.Printf("Total Documents: %d\n", status.TotalDocuments)
	fmt.Printf("Needs Embedding: %d\n", status.NeedsEmbedding)
	fmt.Printf("Collections: %d\n", len(status.Collections))

	if len(status.Collections) > 0 {
		fmt.Println("\nCollections:")
		for _, name := range status.Collections {
			fmt.Printf("  - %s\n", name)
		}
	}

	return nil
}

func outputStatusMarkdown(status mmq.Status) error {
	fmt.Println("# MMQ Status\n")
	fmt.Printf("**Database:** %s  \n", status.DBPath)
	fmt.Printf("**Cache:** %s  \n", status.CacheDir)
	fmt.Printf("**Documents:** %d  \n", status.TotalDocuments)
	fmt.Printf("**Needs Embedding:** %d  \n", status.NeedsEmbedding)
	fmt.Printf("**Collections:** %d\n\n", len(status.Collections))

	if len(status.Collections) > 0 {
		fmt.Println("## Collections\n")
		for _, name := range status.Collections {
			fmt.Printf("- %s\n", name)
		}
	}

	return nil
}
