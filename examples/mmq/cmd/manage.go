package cmd

import (
	"fmt"

	"github.com/crosszan/modu/examples/mmq/format"
	"github.com/crosszan/modu/pkg/mmq"
	"github.com/spf13/cobra"
)

// status 命令
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show index status",
	RunE:  runStatus,
}

// update 命令
var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Re-index all collections",
	Long:  "Re-index all collections to update the index with new/modified documents",
	RunE:  runUpdate,
}

// embed 命令
var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "Generate vector embeddings",
	Long:  "Generate vector embeddings for all documents that need them",
	RunE:  runEmbed,
}

var (
	gitPull bool
)

func init() {
	updateCmd.Flags().BoolVar(&gitPull, "pull", false, "Git pull before indexing")
}

func runStatus(cmd *cobra.Command, args []string) error {
	m, err := getMMQ()
	if err != nil {
		return err
	}
	defer m.Close()

	status, err := m.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	return format.OutputStatus(status, format.Format(outputFormat))
}

func runUpdate(cmd *cobra.Command, args []string) error {
	m, err := getMMQ()
	if err != nil {
		return err
	}
	defer m.Close()

	// 获取所有集合
	collections, err := m.ListCollections()
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	if len(collections) == 0 {
		fmt.Println("No collections found")
		return nil
	}

	fmt.Printf("Updating %d collection(s)...\n\n", len(collections))

	totalDocs := 0

	for _, coll := range collections {
		fmt.Printf("Collection: %s\n", coll.Name)

		// TODO: 如果 gitPull，在这里执行 git pull

		// 索引文档
		err = m.IndexDirectory(coll.Path, mmq.IndexOptions{
			Collection: coll.Name,
			Mask:       coll.Mask,
			Recursive:  true,
		})

		if err != nil {
			fmt.Printf("  Error: %v\n\n", err)
			continue
		}

		// 获取更新后的集合信息
		updatedColl, err := m.GetCollection(coll.Name)
		if err == nil {
			fmt.Printf("  Documents: %d\n", updatedColl.DocCount)
			totalDocs += updatedColl.DocCount
		}

		fmt.Println()
	}

	fmt.Printf("Total documents: %d\n", totalDocs)

	// 检查是否需要生成嵌入
	status, _ := m.Status()
	if status.NeedsEmbedding > 0 {
		fmt.Printf("\n%d documents need embeddings. Run 'mmq embed' to generate them.\n", status.NeedsEmbedding)
	}

	return nil
}

func runEmbed(cmd *cobra.Command, args []string) error {
	m, err := getMMQ()
	if err != nil {
		return err
	}
	defer m.Close()

	// 检查是否需要嵌入
	status, err := m.Status()
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if status.NeedsEmbedding == 0 {
		fmt.Println("All documents already have embeddings")
		return nil
	}

	fmt.Printf("Generating embeddings for %d documents...\n", status.NeedsEmbedding)
	fmt.Println("This may take a while...")
	fmt.Println()

	err = m.GenerateEmbeddings()
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	fmt.Println("\n✓ Embeddings generated successfully")
	return nil
}
