package main

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	initDB()
	initPersonaSchema()
	initChatSchema()
	initNotificationsSchema()
	initEvalSchema()
	initCrossroadSchema()
	initSedimentSchema()

	r := gin.Default()

	// CORS 中间件（允许前端开发服务器跨域调用）
	r.Use(corsMiddleware())

	// 静态文件：评估指标证明图片
	r.Static("/uploads/proofs", ProofsDir)

	// API 路由
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})

	// 认证
	api.POST("/auth/register", register)
	api.POST("/auth/login", login)
	api.GET("/auth/me", authMiddleware(), me)

	// 用户
	api.PUT("/users/:id", authMiddleware(), updateUser)
	api.GET("/users/me/skills", authMiddleware(), mySkills)

	// 技能
	api.GET("/skills", listSkills)                 // 搜索/列表（游客可用）
	api.POST("/skills", authMiddleware(), createSkill) // 发布（需登录）
	api.GET("/skills/:id", getSkill)               // 详情（游客可用）
	api.DELETE("/skills/:id", authMiddleware(), deleteSkill) // 删除（仅属主）
	api.GET("/skills/:id/download", downloadSkill) // 下载 zip（游客可用）
	api.GET("/skills/:id/explain", authMiddleware(), explainSkill) // AI 个性化解读（需登录）

	// AI 引导创建 Skill（需登录）：多模态对话引导 + 生成 skill 包
	api.POST("/skills/guide/chat", authMiddleware(), guideChat)
	api.POST("/skills/guide/generate", authMiddleware(), guideGenerate)

	// 评分 / 评价（需登录提交，游客可看列表）
	api.POST("/skills/:id/reviews", authMiddleware(), submitReview)
	api.GET("/skills/:id/reviews", optionalAuth(), listReviews)

	// Issue 反馈（类 GitHub issue）
	api.POST("/skills/:id/issues", authMiddleware(), createIssue)
	api.GET("/skills/:id/issues", listIssues)
	api.PATCH("/issues/:id", authMiddleware(), closeIssue)

	// 论坛（Forum）：不能成为 Skill 的经验、询问、寻找没有 Skill 的地方。
	// 前台搜索路由不到 Skill 时的出口。游客可读，登录可发帖与回复。
	api.GET("/forum/topics", listForumTopics)
	api.POST("/forum/topics", authMiddleware(), createForumTopic)
	api.GET("/forum/topics/:id", getForumTopic)
	api.POST("/forum/topics/:id/replies", authMiddleware(), createForumReply)
	api.POST("/forum/topics/:id/like", authMiddleware(), likeTopic)
	api.POST("/forum/replies/:id/like", authMiddleware(), likeReply)

	// 虚拟自己（Persona）：引导对话保留/蒸馏/扮演/权限
	api.POST("/persona/conversations", authMiddleware(), saveConversation)
	api.POST("/persona/conversations/:id/distill", authMiddleware(), distillConversation)
	api.GET("/persona/me", authMiddleware(), getMyPersona)
	api.PATCH("/persona/me", authMiddleware(), updateMyPersona)
	api.GET("/persona/public/:userId", optionalAuth(), getPublicPersona)
	api.POST("/persona/public/:userId/chats", authMiddleware(), createPersonaChat)
	api.POST("/persona/chat/:chatId/messages", authMiddleware(), sendPersonaChatMessage)
	api.GET("/persona/chat/:chatId/messages", authMiddleware(), getPersonaChatMessages)
	api.GET("/persona/me/chats", authMiddleware(), listMyPersonaChats)

	// 在线聊天（Direct Chat）：轮询实现
	api.POST("/chat/direct", authMiddleware(), createDirectChat)
	api.GET("/chat/direct", authMiddleware(), listDirectChats)
	api.POST("/chat/direct/:id/messages", authMiddleware(), sendDirectMessage)
	api.GET("/chat/direct/:id/messages", authMiddleware(), getDirectMessages)

	// 消息通知（Notifications）：铃铛角标 + 通知列表
	api.GET("/notifications", authMiddleware(), listNotifications)
	api.GET("/notifications/unread-count", authMiddleware(), unreadNotifications)
	api.POST("/notifications/read", authMiddleware(), markNotificationsRead)
	api.POST("/notifications/:id/read", authMiddleware(), markNotificationRead)
	}

	// ---------- 成长闭环（PRD P0）----------
	// 主张：做事发生在平台内 → 供给是执行的副产品 → 信任来自证据而非评分
	growth := r.Group("/api/growth")
	{
		// 首页 Wow 多轮对话：意图识别 Agent（写论文/画图等 → 调出相关 skill 卡片）
		growth.POST("/wow/discuss", authMiddleware(), handleWowDiscuss)

		// F1 目标识别与四筛判定（伪需求在这里被拦住，不进任务流）
		growth.POST("/goals/interpret", authMiddleware(), interpretGoal)

		// F4 任务工作台：所有执行必须落 execution_steps
		growth.POST("/executions", authMiddleware(), createExecution)
		growth.GET("/executions", authMiddleware(), listMyExecutions)
		growth.GET("/executions/:id", authMiddleware(), getExecution)
		growth.POST("/executions/:id/advance", authMiddleware(), advanceExecution)
		growth.POST("/executions/:id/decide", authMiddleware(), recordDecision)
		growth.POST("/executions/:id/edit", authMiddleware(), recordEdit)
		growth.POST("/executions/:id/complete", authMiddleware(), completeExecution)
		growth.POST("/executions/:id/abandon", authMiddleware(), abandonExecution)
		// 收尾闭环：两问 + verdict 由 LLM 按 skill 文档生成，改进建议手动输入
		growth.POST("/executions/:id/closing/questions", authMiddleware(), genClosingQuestions)
		growth.POST("/executions/:id/closing/submit", authMiddleware(), submitClosing)

		// F5 Skill Creator：轨迹 → 四槽 → 蒸馏度 → 六 slot 文件夹
		growth.POST("/executions/:id/distill", authMiddleware(), distillExecution)
		growth.GET("/drafts/:versionID", authMiddleware(), getDraft)
		growth.PATCH("/drafts/:versionID", authMiddleware(), updateDraft)
		growth.POST("/drafts/:versionID/decisions", authMiddleware(), upsertDecision)
		growth.DELETE("/decisions/:id", authMiddleware(), deleteDecision)
		growth.POST("/drafts/:versionID/downgrade", authMiddleware(), downgradeToInsight)
		growth.POST("/drafts/:versionID/generate-folder", authMiddleware(), generateFolder)

		// F6 发布前四问与门禁
		growth.POST("/skills/:id/evals/run", authMiddleware(), runEvals)
		growth.GET("/skills/:id/gate", getGateStatus)
		growth.POST("/skills/:id/publish", authMiddleware(), publishSkill)

		// F6b 门禁失败反馈 + 反向指导：逐条失败原因 + 生成可写回草稿的修复建议
		growth.POST("/skills/:id/gate-fix-suggestion", authMiddleware(), gateFixSuggestion)
		growth.POST("/skills/:id/gate-apply-fix", authMiddleware(), gateApplyFix)

		// F7/F8 准入四层与两段式路由
		growth.POST("/route", authMiddleware(), routeSkills)
		growth.POST("/admin/recompute-scores", authMiddleware(), recomputeAllScores)

		// F10 Trust Card 与判断级溯源
		growth.GET("/skills/:id/trust-card", getTrustCard)
		growth.GET("/decisions/:id/trace", getDecisionTrace)

		// F5.3b 轨迹补录：承认用户会在平台外做事，但蒸馏度封顶 0.85
		growth.POST("/backfill", authMiddleware(), backfillExecution)

		// 沉淀双通道（v1.3）：AI 访谈多轮口述 + 上传 Skill 包（每次 LLM 四维评测）
		growth.POST("/sediment/chat", authMiddleware(), sedimentChat)
		growth.POST("/sediment/finish", authMiddleware(), sedimentFinish)
		growth.POST("/sediment/upload", authMiddleware(), sedimentUpload)
		growth.GET("/sediment/evals/:skillID", authMiddleware(), getSedimentEval)

		// F17 编排态：长周期方向性需求。只承诺编排，不承诺结果。
		// probe 与 interview 单独命名，避免与 /orchestrations/:id 在同级产生静态段与参数段冲突
		growth.POST("/orch-probe", authMiddleware(), probeOrchestration)
		growth.POST("/orch-interview", authMiddleware(), interviewOrchestration)
		growth.POST("/orchestrations", authMiddleware(), createOrchestration)
		growth.GET("/orchestrations", authMiddleware(), listMyOrchestrations)
		growth.GET("/orchestrations/:id", authMiddleware(), getOrchestration)
		growth.POST("/orchestrations/:id/adopt", authMiddleware(), adoptOrchestration)
		growth.PATCH("/orchestrations/:id/items/:itemId", authMiddleware(), updateOrchItem)
		growth.POST("/orchestrations/:id/reviews", authMiddleware(), reviewOrchestration)

		// F13 个人成长主页与成长身份（成长路径从真实执行派生）
		// 注意：my-profile 与 profile/:id 分开命名，避免静态段与参数段在同级冲突
		growth.GET("/my-profile", authMiddleware(), getMyGrowthProfile)
		growth.PATCH("/my-profile/visibility", authMiddleware(), updateVisibility)
		growth.GET("/profile/:id", optionalAuth(), getUserGrowthProfile)

		// F12 反馈闭环与版本升级
		growth.POST("/executions/:id/feedback", authMiddleware(), submitExecFeedback)
		growth.GET("/skills/:id/version-candidates", listVersionCandidates)
		growth.POST("/version-candidates/:id/accept", authMiddleware(), acceptVersionCandidate)

		// 评测平台（AI Skill 自动化评测管道）
		growth.POST("/eval/skills/:id/pipeline", authMiddleware(), startEvalPipeline)      // 触发完整管道
		growth.GET("/eval/skills/:id/report", getEvalReport)                               // 评测进度与报告
		growth.GET("/eval/skills/:id/human-review", getHumanReviewCases)                   // 待人工复核项
		growth.POST("/eval/human-review/submit", authMiddleware(), submitHumanReview)      // 人工复核提交
		growth.GET("/eval/skills/:id/contract", getContract)                               // 查看契约与环境
		growth.POST("/eval/skills/:id/contract", authMiddleware(), saveContractHandler)    // 保存契约（触发用例重生成）
		growth.POST("/eval/test-cases/generate", authMiddleware(), previewTestCases)       // 契约→预览测试用例
	}

	// ---------- 迷茫期路由器（Crossroad）----------
	// 主张：每一次「我不知道」，都有人真走过。三条纪律：不测评、不建议、不承诺。
	// S1 入口对话是唯一入口：一句话 → A1 四态路由 → 探索流 / 路口页 / 编排态。
	crossroad := r.Group("/api/crossroad")
	{
		// S1 入口 + A1 路由器：一句话分类四态（情绪类拦截不落库）
		crossroad.POST("/moments", authMiddleware(), routeMoment)
		crossroad.GET("/moments", authMiddleware(), listMyMoments)
		crossroad.GET("/moments/:momentId", authMiddleware(), getMoment)

		// S2 探索流：A2 迷茫访谈（≤5 轮，只问经历）+ A3 假设生成（原话溯源）
		crossroad.POST("/interviews/:momentId/turn", authMiddleware(), nextInterviewTurn)
		crossroad.GET("/interviews/:momentId", authMiddleware(), getInterviewSnapshot)
		crossroad.POST("/moments/:momentId/hypotheses", authMiddleware(), generateHypotheses)
		crossroad.GET("/moments/:momentId/hypotheses", authMiddleware(), func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"data": listHypotheses(c.Param("momentId"))})
		})

		// S2 选卡：A4 规则匹配（非 AI，宁缺毋滥）
		crossroad.POST("/moments/:momentId/match", authMiddleware(), matchCards)

		// S6 学长录入端：A5 四槽抽取（无来源即丢弃；boundary 为空只存草稿）
		crossroad.POST("/seniors/transcribe", authMiddleware(), transcribeSenior)
		crossroad.PATCH("/cards/:id", authMiddleware(), patchCard)
		crossroad.GET("/cards", authMiddleware(), listCards)
		crossroad.GET("/cards/:id", authMiddleware(), getCard)

		// S3 微尝试：选卡即装载成陪跑 Agent（卡就是 Skill 包，不重新生成人格）
		crossroad.POST("/attempts", authMiddleware(), startAttempt)
		crossroad.GET("/attempts", authMiddleware(), listMyAttempts)
		crossroad.GET("/attempts/:id", authMiddleware(), getAttempt)
		crossroad.POST("/attempts/:id/chat", authMiddleware(), coachTurn)
		crossroad.POST("/attempts/:id/complete", authMiddleware(), completeAttempt)
		crossroad.POST("/attempts/:id/abandon", authMiddleware(), abandonAttempt)

		// 许愿池：没有匹配卡时的出口，反向指导供给
		crossroad.POST("/wishes", authMiddleware(), addWish)
		crossroad.GET("/wishes", authMiddleware(), listWishes)

		// S5 路口分叉：抉择型只看走过的人（绝对人数 + 代价原话，不给百分比）
		crossroad.GET("/forks", authMiddleware(), listForks)
		crossroad.POST("/forks", authMiddleware(), upsertFork)

		// S4 探索地图：A6 地图叙事（禁评价词）
		crossroad.GET("/map", authMiddleware(), myCrossroadMap)
		crossroad.POST("/map/narrate", authMiddleware(), narrateMap)
	}

	port := os.Getenv("SKILLHUB_PORT")
	if port == "" {
		port = "8080"
	}

	r.Run(":" + port)
}

// corsMiddleware 允许前端跨域请求
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
