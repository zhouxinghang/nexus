# Nexus 个人工作台：业界参照与候选优化

调研日期：2026-10-14（Asia/Shanghai）  
性质：研究建议，**不是已确认 spec，不构成范围变更或实现承诺**。

## 结论摘要

Nexus 不需要先从五个工具扩张到十几个工具。更值得借鉴的是成熟产品如何缩短「记录 → 找到 → 行动 → 回看」的路径，以及如何保证个人数据可迁移、可恢复。

优先考虑：

1. **统一入口**：跨模块搜索、键盘操作、对象直达链接、全局快速创建。
2. **面向日常的视图**：Tasks 的今天/未来七天/逾期筛选；Bookmarks 的检索与收藏入口；Journal 的按日期回看。
3. **数据可信与可带走**：备份和恢复演练、开放格式导出、破坏性操作的可解释结果。
4. **已有模块的深度打磨**：Files 的定位与预览、Journal 的提交前草稿、监控数据的时效性和故障上下文。

这些是从第一方能力抽象出的 Nexus 候选方案，不表示对标产品实现了同一个「五合一工作台」，也不表示 Nexus 应复制其全部模型。

## 方法与证据边界

- 直接读取 7 个产品的官方文档或官方 GitHub 仓库；没有使用二手榜单，也没有通过搜索引擎做全市场穷举。
- 检索方式是 HTTPS 获取文档和源码；不是安装试用或浏览器验收，没有性能实测，官方营销中的性能承诺不作为结论依据。
- 下文「事实」只描述成功读取的第一方资料；「建议」是结合 Nexus 边界的推断。
- GitHub 文档使用当前分支，内容可能后续变化；本文记录的是本次读取结果，不声称对应某个固定发行版。
- 部分目录发现请求遇到 GitHub API 限流，个别猜测文档路径返回 404；这些未作为证据。关键结论均有已成功读取的页面或源码。

## Nexus 的约束先于对标

依据 [CONTEXT.md](/Users/bytedance/workspace/my-nexus/CONTEXT.md) 和 [ADR 目录](/Users/bytedance/workspace/my-nexus/docs/adr)：

- 单 Owner、单 Host、原生 Go 进程；多机器 agent 与团队权限不是遗漏。
- Overview 是只读聚合，不保存自己的业务数据，且不发送通知。全局快捷操作可以放在应用外壳，不要塞入 Overview 的汇总职责。
- Tasks 以 Labels 为唯一分组，不引入 projects/lists、任意工作流或任务树。
- Journal Entry 是带创建时间的不可编辑、不可删除、追加式纯文本记录。草稿不是已提交的 Journal Entry。
- Files 的 Resource 指向 Managed Root 内的真实文件；外部网站仍归 Bookmarks，不能把二者混成一种 Resource。
- Bookmark Health 表示当前约定下的 HTTP 可达性，不能偷换成完整业务可用性；ADR-0005 当前规定 `<500` 为 `up`，固定周期检查。

当前已有的标签、子任务、归档、拖拽、到期日、书签元数据/点击次数/健康检查、Host 历史曲线和 Journal 搜索等，不应仅因对标产品也有就重新列为「缺失功能」。具体实现差距另做本地代码核验。

## 七个产品：事实与适用模式

| 产品 | 已核实的一方事实 | 适合 Nexus 的借鉴 | 不宜直接照搬 |
|---|---|---|---|
| Homepage | Quick Launch 可检索服务、进行网页搜索或打开 URL；支持将描述纳入检索；提供移动端快捷入口配置。[S1] | 一个统一、可键盘操作的入口，比再加若干 Overview 卡片更有使用频次价值。 | 大量外部服务集成；将个人内容搜索默认发给公网搜索引擎。 |
| Dashy | 可按 title、URL、description、alias、tags 检索；支持键盘遍历和启动；支持 URL alias、隐藏低频入口但保留检索。[S2] | Bookmarks 应兼顾「少量常用直接打开」与「大量内容搜索找到」；可以有对象直达链接。 | 多 Owner、SSO 权限矩阵、工作区 iframe、多页面复杂布局系统。 |
| Vikunja | 官方功能页列出 Quick Add、Saved Filters、到期日及查看即将到期任务；还支持重复任务、提醒、项目和协作。[S3] | 快速创建和可保存的筛选条件；用现有 due date/status/Labels 生成「今天」「未来七天」「逾期」。 | projects/subprojects、团队协作、复杂任务关系；重复任务/提醒不能未经范围确认自动引入。 |
| Memos | 官方 README 描述无须先选标题/目录的快速记录、按日回看、搜索/筛选、可保存视图，以及完整导出。[S4] | 低摩擦记录、时间轴定位和导出；强化 Journal 的「写下—找回」。 | 可变笔记本、公开分享、多 Owner、附件和富文本编辑器；不能借鉴成允许编辑历史 Journal Entry。 |
| Obsidian | 官方文档说明笔记以本地 Markdown 纯文本保存；元数据缓存可重建；内部链接可定位文件/标题/块；Templates 可插入预定义文本和日期时间。[S5][S6][S7] | 数据可带走、索引与事实源分离；稳定直达链接；提交前纯文本模板。 | 重写 Nexus 为文件型数据库、整套知识图谱、社区插件运行时；内部链接变更不得反向改写历史 Journal Entry。 |
| linkding | 官方 README 明确 tags、批量编辑、稍后读、Netscape HTML 导入导出、浏览器扩展/bookmarklet；扩展文档支持快速新增与地址栏检索。[S8][S9] | Bookmarks 先改善搜索、固定常用、导入导出及低摩擦收集；如有阅读需求，再考虑稍后读。 | 网页全文归档、Internet Archive 写入、全文抓取等重量功能不应成为导航板的默认职责。 |
| Uptime Kuma | README 包含 HTTP 等探测、通知、延迟图与证书信息；源码可核实重试次数、重试间隔、维护状态、持续失败重发配置，默认配置中重发间隔为 0。[S10][S11][S12] | 让异常可解释：最近检查、延迟、历史变化、失败原因；如果未来加告警，要单独设计降噪和恢复语义。 | 完整监控平台、众多协议、公开状态页、多通知渠道；不能因其存在而突破 Nexus 的单 Host 或 Overview 永不通知约束。 |

## 候选能力：按价值收敛，而非按产品堆叠

### A. 统一搜索 + 快速操作：最值得优先验证

**依据**：Homepage/Dashy 的键盘搜索与启动；Vikunja 的 Quick Add。[S1][S2][S3]

**建议**：

- 全局 `Cmd/Ctrl+K`：先提供页面跳转、已有对象搜索、创建 Task/Journal Entry/Bookmark；不执行未确认的删除或 Host 操作。
- 结果明确标注类型，直达对应对象；先检索标题、名称、路径、已有文本字段，不先上 AI 或外部搜索服务。
- 全局搜索只是分发和聚合，不把 Task、Resource、Journal Entry、Bookmark 改造成同一种实体。Journal 继续保留当前中英混合子串匹配语义。
- 空结果要允许「去对应模块创建」，但创建动作属于各模块/全局外壳，Overview 仍然只读。
- 不自动把 Journal 或文件内容作为公网搜索词发送；外部搜索若加入，需要与本地检索明显分开。

**注意**：小规模可先复用各模块查询能力；是否需要统一索引，应由实际数据量与延迟测量决定，不预设 Elasticsearch 等新基础设施。

### B. 今日工作视图，而不是再造项目管理系统

**依据**：Vikunja 的到期任务视图和 Saved Filters。[S3]

**建议**：

- Tasks 增加「今天」「未来七天」「逾期」「无日期」快捷筛选，可组合 Label、状态、优先级。
- Saved View 只保存查询条件，不等于新的项目/列表归属；同一个 Task 可以自然出现在多个结果中。
- 保留现有看板内拖拽顺序；如果另做按到期日排列的只读结果视图，应清楚区分，避免悄悄改变看板排序规则。
- Overview 可以展示上述计数并链接到已筛选的 Tasks 页面，但不替代完整任务操作界面。

**暂不默认加入**：重复任务、提醒、日历双向同步、自然语言日期解析。前两项涉及新的时间与通知语义，需要另行确认范围；固定表单快捷创建即可先获得大部分收益。

### C. Bookmarks：从「陈列链接」到「快速到达」

**依据**：Dashy 的搜索/低频隐藏、linkding 的收集与互操作。[S2][S8][S9]

**建议**：

- 先做名称/域名搜索、Owner 主动置顶/排序；点击次数只提供参考，不能完全替代 Owner 的选择。
- 如果书签规模足够大，再加入轻量分组或标签，避免一开始同时引入两套分类机制。
- 支持浏览器常见 Netscape HTML 导入导出；导入前预览数量、冲突/重复项处理结果，不直接覆盖 Owner 自定义名称和图标。
- 新增一个可预填 URL/标题的创建入口，先验证 bookmarklet 或复制粘贴流程，再决定是否值得维护完整浏览器扩展。
- 浏览器采集凭据不得嵌在分享 URL 中，仍须走 Owner 已认证流程。

**选做**：「稍后读」适合确有阅读积压的 Owner；它不是导航板必备。全文归档需要额外存储、解析、安全和失败处理，不列为首批。

### D. Journal：改善提交前与读取端，不破坏历史

**依据**：Memos 的按日回看/导出、Obsidian 的模板能力。[S4][S7]

**建议**：

- 补充提交前草稿、离开提示；现有 `Cmd/Ctrl+Enter` 提交和提交失败时保留正文应继续保留，不重复立项。持久草稿若保存于浏览器，应说明隐私和清理策略。
- 日期范围筛选、跳到某天、按天分隔结果；继续支持现有子串检索，而不是重复建设搜索。
- 可选纯文本模板，如「完成了什么 / 遇到的问题 / 下一步」。模板只辅助生成新记录，不回填或改写旧记录。
- 导出为文本/Markdown/JSON 时保留原始正文、创建时间和稳定标识；更正内容用新的 Journal Entry 追加说明。

**不建议**：编辑/删除、富文本重写、将记录拆成可编辑知识库；这些不是界面小优化，而是改变 ADR-0003。

### E. 数据可恢复性：不显眼，但应早于重量新功能

**依据**：linkding 的标准格式导入导出、Memos 的完整导出、Obsidian 的本地文本与可重建缓存。[S4][S5][S8]

**建议，属于 Nexus 工程推断**：

- 明确区分「导出方便迁移」和「备份可恢复实例」。Netscape HTML 书签导出不能替代数据库备份。
- 给出数据库、实例配置和 Managed Roots 原始文件各自的备份范围；仅备份资源元数据不能恢复被删的实际文件。
- 提供版本信息、恢复步骤和一次可重复的恢复演练；灾难恢复优先验证到新实例，避免把恢复流程误做成可编辑历史 Journal 的常规 API。
- 恢复检查至少涵盖 Journal 正文/时间、Task/Label/Subtask 关系、Bookmarks Owner 覆盖值、Resource 元数据与真实路径匹配。
- 升级前备份、导入前预览、失败时给出部分成功明细，比增加一套主题或插件市场更有长期价值。

### F. 现有工具的深度优化：先确认缺口，再立项

以下为本地产品设计推断，不冒称七款产品都具备：

- **Files**：按名称/路径定位、Managed Root 内目录导航、安全的常见格式预览、Missing 原因和扫描反馈、批量操作结果/审计可见。任何预览和导航仍要遵守 Managed Root 边界；不可直接渲染不受信任的活动 HTML。
- **稳定直达链接**：搜索结果、Task 或 Resource 详情、Journal Entry 都能复制内部链接；对象移动或改名后尽量以稳定 ID 定位。先做导航能力，不急着建立跨模块关联表或统一知识图谱。[S6] 是设计参照，不意味着照搬 Obsidian 的路径改写行为。
- **Host/Bookmarks 监控**：清楚显示最后采样时间、数据是否过期、加载失败和未知状态；不要把旧数据留在界面上却表现得像实时数据。当前已有的历史时间范围与采样桶应复用，不再把「历史曲线」当作新需求。
- **健康语义**：展示现有 Health 判定依据；401/404 在当前 `<500` 规则中仍可能是 `up`，不要向 Owner 暗示业务一定正常。新增重试、阈值、证书提醒或报警都应独立评估，不能静默改变 ADR-0005。
- **交互质量**：无鼠标可完成核心操作；焦点管理、移动端适配、错误/空态/部分失败处理一致。仅看源码不能证明这些体验达标，必须另做浏览器验收。

## 不建议为了「行业齐全」而做

- 团队、角色、审批、项目层级、多 Host agents：不符合当前定位。
- 在 Overview 加任务自动化、数据归属或通知引擎：越过既有职责边界。
- 可编辑/可删除 Journal、复杂 Markdown 知识库：与不可变纯文本记录模型冲突。
- 大规模插件市场、任意脚本执行、内置终端、远程控制：会显著扩大原生进程对 Host 的攻击面与维护成本。
- 默认云同步/外部 AI 阅读全部个人数据：不是统一搜索的必要条件；隐私、成本和数据边界需先单独决定。
- RSS、天气、番茄钟、AI 对话、几十种集成：可能对某个 Owner 有价值，但本次未获得比核心使用路径更优先的证据。

## 本地代码核验：哪些是真缺口，哪些已经做了

核验方式：读取当前工作区源码、CONTEXT、ADR 和已有 spec；未启动应用、未跑浏览器或负载测试。以下区分可直接看到的实现事实与建议，性能收益不作数值承诺。

### 已有能力，避免重复建设

- **Overview**：已有五个工具的只读汇总、到期/逾期计数与阈值标色，不是缺少入口页。见 [Overview.tsx](/Users/bytedance/workspace/my-nexus/web/src/pages/Overview.tsx)。
- **Tasks**：已有优先级、到期日、Labels、Subtasks、拖拽排序、归档和恢复；缺的是跨字段检索、日常筛选视图与稳定直达。见 [Tasks.tsx](/Users/bytedance/workspace/my-nexus/web/src/pages/Tasks.tsx)。
- **Files**：已有 Managed Roots、扫描、Missing、标签/备注、分页、批量标签/移动/删除；后端已有审计查询和目录前缀过滤，前端尚未提供对应完整操作入口。见 [Files.tsx](/Users/bytedance/workspace/my-nexus/web/src/pages/Files.tsx)、[Files API](/Users/bytedance/workspace/my-nexus/server/internal/httpapi/files_handlers.go)、[Files 查询](/Users/bytedance/workspace/my-nexus/server/internal/db/queries/files.sql)。
- **Journal**：已有中文/英文子串搜索、游标分页、复制、键盘提交；缺的是日期定位、提交前草稿、模板和导出。见 [Journal.tsx](/Users/bytedance/workspace/my-nexus/web/src/pages/Journal.tsx)。
- **Bookmarks**：已有自动标题/favicon、点击次数、最近打开时间、最常访问排序、健康检查；缺的是检索、主动置顶/分类与标准导入导出。见 [Bookmarks.tsx](/Users/bytedance/workspace/my-nexus/web/src/pages/Bookmarks.tsx)。
- **Servers**：已有 1h/24h/7d/30d 时间范围、服务端查询时分桶、分区当前用量、进程/容器详情，不能再笼统提议“加历史监控”。见 [Servers.tsx](/Users/bytedance/workspace/my-nexus/web/src/pages/Servers.tsx)、[Servers API](/Users/bytedance/workspace/my-nexus/server/internal/httpapi/servers_handlers.go)。
- **基础体验/安全**：已有明暗主题、移动端导航、Toast、登录限流和 Secure/HttpOnly/SameSite Cookie；优化应在这些基础上做，不是从零补齐。

### 建议先处理的实现问题

| 优化项 | 当前证据 | 建议与验收方向 |
|---|---|---|
| 文件筛选的完整性 | [Files.tsx:200](/Users/bytedance/workspace/my-nexus/web/src/pages/Files.tsx#L200) 的类型筛选、数量统计只遍历已加载的 `resources`；服务端分页后再做本地筛选。 | 将类型过滤和计数放入服务端分页查询；验收“目标类型只存在于后续页”也能被找到，避免将“已加载中没有”误解为“整个 Managed Root 没有”。 |
| 网络指标的语义 | [real.go:126](/Users/bytedance/workspace/my-nexus/server/internal/host/real.go#L126) 读取 `BytesRecv/BytesSent` 累计计数；[Servers.tsx:205](/Users/bytedance/workspace/my-nexus/web/src/pages/Servers.tsx#L205) 直接将二者求和绘图，当前卡片也显示累计字节。 | 总量与速率分开显示；速率以相邻有效样本差值/时间差计算，单位 B/s，分别显示入站/出站。处理重启、计数器归零、采样缺口；先求速率再做展示聚合，不能把累计字节均值当带宽。 |
| 到期日期一致性 | [Tasks.tsx:120](/Users/bytedance/workspace/my-nexus/web/src/pages/Tasks.tsx#L120) 按时间戳与当前时刻比较标色；`dueLabel` 按浏览器日历日；[TaskSummary](/Users/bytedance/workspace/my-nexus/server/internal/db/queries/tasks.sql#L159) 按数据库日历日统计；表单又将日期转为 UTC 时间戳。 | 先明确到期日是“日期”而非“精确时刻”，统一 Owner 时区、今日/逾期边界。增加 UTC、Asia/Shanghai、负偏移时区和跨午夜测试，避免“Today”文字与逾期色/Overview 计数矛盾。 |
| 实时状态与失败可见性 | [Servers.tsx:183](/Users/bytedance/workspace/my-nexus/web/src/pages/Servers.tsx#L183) 定时请求；`Live` 标识没有随请求成功、失败和数据年龄变化。后台 [sampler](/Users/bytedance/workspace/my-nexus/server/internal/sampler/sampler.go) 与 [checker](/Users/bytedance/workspace/my-nexus/server/internal/checker/checker.go) 的运行循环忽略返回错误。 | 展示 `sampled_at`、最后成功时间、过期/离线/重试状态；后台记录失败日志和最近成功状态，失败不能表现为正常刷新。不要新增 Overview 通知机制。 |
| 请求生命周期 | [api.ts](/Users/bytedance/workspace/my-nexus/web/src/lib/api.ts) 无取消/超时参数，401 只抛错；[auth.tsx](/Users/bytedance/workspace/my-nexus/web/src/lib/auth.tsx) 只在挂载时检查会话。各页重复实现加载/错误/竞态处理。 | 统一取消旧请求、防止旧响应覆盖新筛选、加载/错误/重试语义；401 更新认证状态并保留可恢复输入。读取可有限重试，写入不应无条件自动重试；Journal 等创建动作先考虑幂等。隐藏页面暂停轮询。 |
| 重复 Host 采集 | [Overview API](/Users/bytedance/workspace/my-nexus/server/internal/httpapi/overview_handlers.go)、[Servers API](/Users/bytedance/workspace/my-nexus/server/internal/httpapi/servers_handlers.go)、后台 sampler 均直接采集 Host。 | 先测量开销，再考虑共享短时采样快照/合并并发采集。保留“实时采样”与“30 秒历史入库”不同节奏，不能简单让所有页面退化为 30 秒陈旧快照。 |
| 无障碍和触屏 | [Modal](/Users/bytedance/workspace/my-nexus/web/src/components/ui.tsx#L246) 有 Escape 和 `aria-modal`，没有完整的焦点循环/返回焦点及标题关联；Tasks/Subtasks 排序使用 HTML 拖拽。 | 按 WAI APG 完成弹窗焦点管理、可访问名称；为排序提供键盘和触屏可用替代操作。已有状态下拉不等于已有非拖拽排序。需浏览器和键盘验收。[S13] |
| 自托管的外部依赖 | [index.css:1](/Users/bytedance/workspace/my-nexus/web/src/index.css#L1) 使用 Google Fonts；[Bookmarks.tsx:78](/Users/bytedance/workspace/my-nexus/web/src/pages/Bookmarks.tsx#L78) 保留 Google favicon 回退。 | 优先用系统字体或打包本地字体；favicon 失败时可直接用首字母，减少第三方请求和域名披露。真正缓存外部图标另评估大小限制、来源限制和安全策略。 |

### Files 的安全基线要与新增功能同步评估

当前操作会修改 Host 的真实文件，且“永久删除、没有回收站”是原 spec 的明确选择，不应当成漏实现。建议优先补充同名目标覆盖保护、批量操作失败明细、磁盘操作成功但索引更新失败时的恢复路径，以及 Managed Root 软链接边界验证。这里只做静态风险识别，**不是完成了渗透测试或证明所有边界已失守**。

审计表/API 已存在，应先补可查询的 UI 和分页；不要重新建设第二份审计记录，也不要自动把全部系统行为写入 Owner 的 Journal。若要回收站，须明确修订 [v1 spec 的永久删除约定](/Users/bytedance/workspace/my-nexus/.scratch/nexus-v1/spec.md)。

## 建议实施顺序与最小验收

下表是候选排序，不是已经批准的排期；每一批都可单独决定是否立项。

| 顺序 | 聚焦交付 | 最小验收 |
|---|---|---|
| 1：可信基础 | 文件筛选/网络单位/到期日/数据过期状态；备份与恢复操作说明；文件变更安全基线。 | 后续页文件能被筛选；流量总量与速率不混淆；Tasks 与 Overview 日期一致；采样失败明确可见；从备份恢复到隔离的新实例并核对数据。 |
| 2：统一入口 | 对象稳定直达链接 + 本地统一搜索 + `Cmd/Ctrl+K`。 | 无须先知道对象属于哪个模块就能检索；命中后直达对应对象；浏览器后退、刷新、复制链接可用；搜索词不默认外发。 |
| 3：每日使用路径 | 全局快速创建 + Journal 提交前草稿 + Tasks 今天/未来七天/逾期视图。 | 不必先打开工具页面才能开始记录；意外离开不丢未提交输入；Overview 计数可链接到匹配的 Tasks 结果；Labels 仍是唯一归属分组。 |
| 4：按使用量加深 | Bookmarks 搜索置顶/导入导出；Files 目录导航/安全预览/扫描进度/审计入口；Journal 日期回看与导出。 | 浏览器书签可预览后导入；文件不用靠手输绝对路径定位；扫描有进度、完成/失败结果；指定日期的记录可定位并以开放格式导出。 |
| 暂缓 | 复杂跨模块关联模型、周期任务与通知、PWA 离线写入、AI 语义检索、网页全文归档。 | 先有明确使用场景，再单独确认范围和数据安全；不要为了“行业都有”而引入。 |

跨模块联动可先以**稳定内部链接**验证需求：在 Task 中引用 Resource/Bookmark，写一条新的 Journal Entry 记录进展；不先统一所有实体、不改写历史 Journal，也不必先建知识图谱。如要结构化反向引用，再单独确定删除、Missing、归档的展示规则。

本次仅新增这份研究文件，未改业务代码、未创建实施 issues、未改变 ADR/CONTEXT。

## 第一方来源索引

- **S1 — Homepage, Settings / Quick Launch**：[官方文档](https://gethomepage.dev/configs/settings/#quick-launch)；[本次读取的 Markdown 源码](https://raw.githubusercontent.com/gethomepage/homepage/main/docs/configs/settings.md)。
- **S2 — Dashy, Search & Shortcuts**：[官方仓库文档](https://github.com/Lissy93/dashy/blob/master/docs/searching.md)。
- **S3 — Vikunja, Features**：[官方功能页](https://vikunja.io/features/)。
- **S4 — Memos, README / Why Memos**：[官方仓库](https://github.com/usememos/memos#why-memos)。
- **S5 — Obsidian, How Obsidian stores data**：[官方帮助源码](https://github.com/obsidianmd/obsidian-help/blob/master/en/Files%20and%20folders/How%20Obsidian%20stores%20data.md)。
- **S6 — Obsidian, Internal links**：[官方帮助源码](https://github.com/obsidianmd/obsidian-help/blob/master/en/Linking%20notes%20and%20files/Internal%20links.md)。
- **S7 — Obsidian, Templates**：[官方帮助源码](https://github.com/obsidianmd/obsidian-help/blob/master/en/Plugins/Templates.md)。
- **S8 — linkding, README / Feature Overview**：[官方仓库](https://github.com/sissbruecker/linkding#introduction)。
- **S9 — linkding, Browser Extension**：[官方文档](https://linkding.link/browser-extension/)。
- **S10 — Uptime Kuma, README / Features**：[官方仓库](https://github.com/louislam/uptime-kuma#-features)。
- **S11 — Uptime Kuma, monitor 状态/重试/维护逻辑**：[官方源码](https://github.com/louislam/uptime-kuma/blob/master/server/model/monitor.js)。
- **S12 — Uptime Kuma, 配置界面与默认值**：[官方源码](https://github.com/louislam/uptime-kuma/blob/master/src/pages/EditMonitor.vue)。
- **S13 — W3C WAI-ARIA APG, Dialog (Modal) Pattern**：[焦点循环、进入/退出焦点与对话框命名](https://www.w3.org/WAI/ARIA/apg/patterns/dialog-modal/)。
