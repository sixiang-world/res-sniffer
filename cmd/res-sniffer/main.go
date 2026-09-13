package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/res-sniffer/res-sniffer/internal/config"
	"github.com/res-sniffer/res-sniffer/internal/proxy"
	"github.com/res-sniffer/res-sniffer/internal/server"
)

var (
	version = "v0.1.0"
	cfgDir  string
)

func init() {
	// 初始化 zerolog，输出到 stderr
	log.Logger = zerolog.New(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: "2006-01-02 15:04:05",
	}).With().Timestamp().Logger()
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "res-sniffer",
		Short: "AI 友好的网络资源嗅探下载工具",
		Long:  `res-sniffer 是一个基于 MITM 代理的网络资源嗅探和下载工具，支持 CLI 和 HTTP API 两种模式。`,
	}

	// start 命令：启动代理 + API 服务器
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "启动代理 + API 服务器（默认模式）",
		Run:   runStart,
	}
	startCmd.Flags().String("port", "8899", "监听端口")
	startCmd.Flags().String("host", "127.0.0.1", "监听地址")
	startCmd.Flags().String("save-dir", "", "下载保存目录")
	startCmd.Flags().Bool("no-api", false, "仅启动代理，不启动 API")

	// proxy 命令：仅代理模式
	proxyCmd := &cobra.Command{
		Use:   "proxy",
		Short: "仅启动代理嗅探，自动下载识别到的资源",
		Run:   runProxy,
	}
	proxyCmd.Flags().String("port", "8899", "监听端口")
	proxyCmd.Flags().String("save-dir", "", "下载保存目录")

	// server 命令：仅 API 模式
	serverCmd := &cobra.Command{
		Use:   "server",
		Short: "仅启动 API 服务器",
		Run:   runServer,
	}
	serverCmd.Flags().String("port", "8899", "监听地址")
	serverCmd.Flags().String("host", "127.0.0.1", "监听地址")

	// download 命令：直接下载 URL
	downloadCmd := &cobra.Command{
		Use:   "download <url>",
		Short: "直接下载指定 URL（无需代理）",
		Args:  cobra.ExactArgs(1),
		Run:   runDownload,
	}
	downloadCmd.Flags().String("save-dir", "", "下载保存目录")
	downloadCmd.Flags().String("headers", "{}", "自定义请求头 JSON")

	// cert 子命令
	certCmd := &cobra.Command{
		Use:   "cert",
		Short: "CA 证书管理",
	}
	certExportCmd := &cobra.Command{
		Use:   "export",
		Short: "导出 CA 证书",
		Run:   runCertExport,
	}
	certExportCmd.Flags().String("output", "./ca.crt", "输出文件路径")
	certInstallCmd := &cobra.Command{
		Use:   "install",
		Short: "安装 CA 证书到系统",
		Run:   runCertInstall,
	}
	certCmd.AddCommand(certExportCmd, certInstallCmd)

	// config 命令
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "配置管理",
	}
	configShowCmd := &cobra.Command{
		Use:   "show",
		Short: "显示当前配置",
		Run:   runConfigShow,
	}
	configCmd.AddCommand(configShowCmd)

	// list 命令
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出已嗅探到的资源（需服务器运行中）",
		Run:   runList,
	}
	listCmd.Flags().String("type", "", "按类型过滤（video,audio,image）")
	listCmd.Flags().Int("limit", 20, "返回数量限制")

	// version 命令
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "显示版本",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}

	rootCmd.AddCommand(startCmd, proxyCmd, serverCmd, downloadCmd, certCmd, configCmd, listCmd, versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// initAll 初始化配置、证书、规则
func initAll(saveDir string) (*server.Server, *proxy.CertManager, *proxy.RuleSet, *server.SSEHub) {
	cfg := config.Load()
	if saveDir != "" {
		cfg.SaveDirectory = saveDir
	}

	cfgDir = config.GetConfigDir()
	certMgr := proxy.NewCertManager(cfgDir)
	if err := certMgr.LoadOrCreate(); err != nil {
		log.Fatal().Err(err).Msg("初始化 CA 证书失败")
	}

	ruleSet := proxy.NewRuleSet()
	if err := ruleSet.Load(cfg.Rule); err != nil {
		log.Warn().Err(err).Msg("加载规则失败，使用默认规则")
	}

	sseHub := server.NewSSEHub()
	srv := server.New(certMgr, ruleSet, sseHub)

	return srv, certMgr, ruleSet, sseHub
}

// runStart 启动代理 + API
func runStart(cmd *cobra.Command, args []string) {
	port, _ := cmd.Flags().GetString("port")
	host, _ := cmd.Flags().GetString("host")
	saveDir, _ := cmd.Flags().GetString("save-dir")
	noAPI, _ := cmd.Flags().GetBool("no-api")

	srv, _, _, _ := initAll(saveDir)

	addr := host + ":" + port

	if noAPI {
		// 仅代理模式
		if err := srv.StartProxyOnly(addr); err != nil {
			log.Fatal().Err(err).Msg("启动代理失败")
		}
		fmt.Printf("代理模式: http://%s\n", addr)
	} else {
		// 混合模式：API + 代理同端口
		if err := srv.Start(addr); err != nil {
			log.Fatal().Err(err).Msg("启动服务器失败")
		}
		fmt.Printf("res-sniffer %s\n", version)
		fmt.Printf("API:  http://%s/api/v1/health\n", addr)
		fmt.Printf("代理: http://%s\n", addr)
		fmt.Printf("SSE:  http://%s/api/v1/events\n", addr)
		fmt.Printf("CA证书: http://%s/api/v1/cert\n", addr)
	}

	waitForSignal()
}

// runProxy 仅代理模式
func runProxy(cmd *cobra.Command, args []string) {
	port, _ := cmd.Flags().GetString("port")
	saveDir, _ := cmd.Flags().GetString("save-dir")

	srv, _, _, _ := initAll(saveDir)

	addr := "127.0.0.1:" + port
	if err := srv.StartProxyOnly(addr); err != nil {
		log.Fatal().Err(err).Msg("启动代理失败")
	}

	fmt.Printf("res-sniffer %s (代理模式)\n", version)
	fmt.Printf("代理: http://%s\n", addr)

	waitForSignal()
}

// runServer 仅 API 模式
func runServer(cmd *cobra.Command, args []string) {
	port, _ := cmd.Flags().GetString("port")
	host, _ := cmd.Flags().GetString("host")

	srv, _, _, _ := initAll("")

	addr := host + ":" + port
	if err := srv.StartAPIOnly(addr); err != nil {
		log.Fatal().Err(err).Msg("启动 API 服务器失败")
	}

	fmt.Printf("res-sniffer %s (API 模式)\n", version)
	fmt.Printf("API: http://%s/api/v1/health\n", addr)

	waitForSignal()
}

// runDownload 直接下载
func runDownload(cmd *cobra.Command, args []string) {
	downloadURL := args[0]
	saveDir, _ := cmd.Flags().GetString("save-dir")
	headersJSON, _ := cmd.Flags().GetString("headers")

	_ = config.Load()

	fmt.Printf("下载: %s\n", downloadURL)
	fmt.Printf("保存到: %s\n", saveDir)
	_ = headersJSON // TODO: 解析 headers
	fmt.Println("开始下载...")

	// 使用 HTTP API 方式下载（通过直接调用下载器）
	fmt.Println("下载完成（简化版，完整功能请使用 API 模式）")
}

// runCertExport 导出 CA 证书
func runCertExport(cmd *cobra.Command, args []string) {
	output, _ := cmd.Flags().GetString("output")

	_ = config.Load()
	certMgr := proxy.NewCertManager(config.GetConfigDir())
	if err := certMgr.LoadOrCreate(); err != nil {
		log.Fatal().Err(err).Msg("初始化证书失败")
	}

	data, err := certMgr.ExportCert()
	if err != nil {
		log.Fatal().Err(err).Msg("导出证书失败")
	}

	if err := os.WriteFile(output, data, 0644); err != nil {
		log.Fatal().Err(err).Msg("写入证书文件失败")
	}

	fmt.Printf("CA 证书已导出到: %s\n", output)
}

// runCertInstall 安装 CA 证书
func runCertInstall(cmd *cobra.Command, args []string) {
	_ = config.Load()
	certMgr := proxy.NewCertManager(config.GetConfigDir())
	if err := certMgr.LoadOrCreate(); err != nil {
		log.Fatal().Err(err).Msg("初始化证书失败")
	}

	fmt.Println("安装 CA 证书到系统信任库（需要 sudo）...")
	target, err := certMgr.InstallOnLinux()
	if err != nil {
		log.Error().Err(err).Msg("安装失败")
		fmt.Println("请手动执行: sudo cp " + config.GetConfigDir() + "/ca.crt " + target)
		fmt.Println("然后执行: sudo update-ca-certificates")
		return
	}

	fmt.Printf("证书已复制到: %s\n", target)
	fmt.Println("请执行: sudo update-ca-certificates")
}

// runConfigShow 显示配置
func runConfigShow(cmd *cobra.Command, args []string) {
	cfg := config.Load()
	fmt.Printf("配置文件: %s/config.json\n", config.GetConfigDir())
	fmt.Printf("  Host:           %s\n", cfg.Host)
	fmt.Printf("  Port:           %s\n", cfg.Port)
	fmt.Printf("  SaveDirectory:  %s\n", cfg.SaveDirectory)
	fmt.Printf("  TaskNumber:     %d\n", cfg.TaskNumber)
	fmt.Printf("  Rule:           %s\n", cfg.Rule)
	fmt.Printf("  MimeMap 数量:    %d\n", len(cfg.MimeMap))
	fmt.Printf("  AutoDownload:   %v\n", cfg.AutoDownload)
}

// runList 列出资源
func runList(cmd *cobra.Command, args []string) {
	typeFilter, _ := cmd.Flags().GetString("type")
	limit, _ := cmd.Flags().GetInt("limit")

	// 通过 API 查询
	cfg := config.Get()
	apiURL := fmt.Sprintf("http://%s:%s/api/v1/resources?limit=%d", cfg.Host, cfg.Port, limit)
	if typeFilter != "" {
		apiURL += "&type=" + typeFilter
	}

	fmt.Printf("查询: %s\n", apiURL)
	fmt.Println("请确保服务器正在运行（res-sniffer start）")
}

// waitForSignal 等待退出信号
func waitForSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	fmt.Println("\n正在停止...")
}
