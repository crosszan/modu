package cmd

import (
	"fmt"
	"os"

	"github.com/crosszan/modu/examples/mmq/format"
	"github.com/crosszan/modu/pkg/mmq"
	"github.com/spf13/cobra"
)

var collectionCmd = &cobra.Command{
	Use:   "collection",
	Short: "Manage collections",
	Long:  "Create, list, rename, and remove collections",
}

var collectionAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a new collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runCollectionAdd,
}

var collectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all collections",
	RunE:  runCollectionList,
}

var collectionRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a collection",
	Args:  cobra.ExactArgs(1),
	RunE:  runCollectionRemove,
}

var collectionRenameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a collection",
	Args:  cobra.ExactArgs(2),
	RunE:  runCollectionRename,
}

var (
	collectionName string
	collectionMask string
	indexNow       bool
)

func init() {
	// collection add 标志
	collectionAddCmd.Flags().StringVarP(&collectionName, "name", "n", "", "Collection name (required)")
	collectionAddCmd.Flags().StringVarP(&collectionMask, "mask", "m", "**/*.md", "File glob pattern")
	collectionAddCmd.Flags().BoolVar(&indexNow, "index", false, "Index documents immediately")
	collectionAddCmd.MarkFlagRequired("name")

	// 添加子命令
	collectionCmd.AddCommand(collectionAddCmd)
	collectionCmd.AddCommand(collectionListCmd)
	collectionCmd.AddCommand(collectionRemoveCmd)
	collectionCmd.AddCommand(collectionRenameCmd)
}

func runCollectionAdd(cmd *cobra.Command, args []string) error {
	path := args[0]

	// 展开路径
	if path[:2] == "~/" {
		homeDir, _ := os.UserHomeDir()
		path = homeDir + path[1:]
	}

	// 检查路径是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	m, err := getMMQ()
	if err != nil {
		return err
	}
	defer m.Close()

	// 创建集合
	err = m.CreateCollection(collectionName, path, mmq.CollectionOptions{
		Mask: collectionMask,
	})
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	fmt.Printf("Created collection '%s' at %s (mask: %s)\n", collectionName, path, collectionMask)

	// 如果指定了索引，立即索引
	if indexNow {
		fmt.Println("Indexing documents...")
		err = m.IndexDirectory(path, mmq.IndexOptions{
			Collection: collectionName,
			Mask:       collectionMask,
			Recursive:  true,
		})
		if err != nil {
			return fmt.Errorf("failed to index documents: %w", err)
		}

		// 获取集合信息
		coll, _ := m.GetCollection(collectionName)
		if coll != nil {
			fmt.Printf("Indexed %d documents\n", coll.DocCount)
		}
	} else {
		fmt.Println("Run 'mmq update' to index documents")
	}

	return nil
}

func runCollectionList(cmd *cobra.Command, args []string) error {
	m, err := getMMQ()
	if err != nil {
		return err
	}
	defer m.Close()

	collections, err := m.ListCollections()
	if err != nil {
		return fmt.Errorf("failed to list collections: %w", err)
	}

	if len(collections) == 0 {
		fmt.Println("No collections found")
		fmt.Println("Use 'mmq collection add <path> --name <name>' to create one")
		return nil
	}

	return format.OutputCollections(collections, format.Format(outputFormat))
}

func runCollectionRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	m, err := getMMQ()
	if err != nil {
		return err
	}
	defer m.Close()

	// 检查集合是否存在
	coll, err := m.GetCollection(name)
	if err != nil {
		return fmt.Errorf("collection not found: %s", name)
	}

	// 确认删除
	fmt.Printf("Remove collection '%s' (%s)?\n", name, coll.Path)
	fmt.Printf("This will remove %d documents from the index.\n", coll.DocCount)
	fmt.Print("Continue? (y/N): ")

	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("Cancelled")
		return nil
	}

	err = m.RemoveCollection(name)
	if err != nil {
		return fmt.Errorf("failed to remove collection: %w", err)
	}

	fmt.Printf("Removed collection '%s'\n", name)
	return nil
}

func runCollectionRename(cmd *cobra.Command, args []string) error {
	oldName := args[0]
	newName := args[1]

	m, err := getMMQ()
	if err != nil {
		return err
	}
	defer m.Close()

	err = m.RenameCollection(oldName, newName)
	if err != nil {
		return fmt.Errorf("failed to rename collection: %w", err)
	}

	fmt.Printf("Renamed collection '%s' to '%s'\n", oldName, newName)
	return nil
}
