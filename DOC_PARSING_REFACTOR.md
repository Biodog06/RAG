# RAG 解析流水线重构技术文档：从 Tika 迁移至 MinerU 与数据工程化

## 1. 背景与现状 (Background)
当前 PaiSmart 项目的文档解析依赖于 **Apache Tika**。虽然 Tika 支持格式众多，但在 RAG (检索增强生成) 场景中存在以下痛点：
*   **PDF 解析乱序**：Tika 无法理解精细的版面布局（Layout），导致文本切块后语义支离破碎。
*   **公式与表格丢失**：PPT 和 PDF 中的关键数学公式和表格结构在 Tika 提取后沦为杂乱字符，严重影响 LLM 的理解。
*   **Excel 检索低效**：将 Excel 强行转为扁平文本并切块，会导致 LLM 无法通过索引定位单元格数据。传统向量检索无法处理需要“聚合计算”的表格问题。

## 2. 目标方案 (The New Blueprint)
为了提升知识库的质量，我们决定对解析流水线进行彻底重构：
1.  **全面去 Tika 化**：剔除陈旧的 Tika 服务，减轻基础设施压力。
2.  **引入 MinerU + gRPC**：针对 PDF、DOCX、PPTX 提供高精度的文档智能解析，输出结构化的 Markdown。
3.  **Excel 数据工程化**：针对表格数据，由“文档解析”转向“数据导入”。利用现有 **MySQL 8** 承载 Excel 行列数据，后续通过 **Text-to-SQL Skill** 进行精准查询。

## 3. 架构设计 (Architecture)

### 3.1 智能解析路由 (Intelligent Routing)
系统根据上传文件的后缀，自动分发至对应的解析管线：

| 后缀 | 管线类型 | 处理核心 | 存储逻辑 |
| :--- | :--- | :--- | :--- |
| `.pdf`, `.docx`, `.pptx`, `.doc`, `.ppt` | **智能文档管线** | MinerU (via gRPC) | 存入 ES 向量库 |
| `.xlsx`, `.xls`, `.csv` | **数据工程管线** | Excelize (Go Native) | 存入 MySQL (JSON/Table) |
| `.txt`, `.md`, 代码文件 | **基础文本管线** | 原生读取 | 存入 ES 向量库 |

### 3.2 跨语言流水线 (Go <-> Python)
由于 MinerU 及其核心引擎 Magic-PDF 基于深度学习栈（Python），我们采用 **gRPC** 作为通信协议。
*   **Go 端**：作为客户端，发送字节流并接收 Markdown。
*   **Python 端**：作为服务端，利用 GPU 加速解析并处理 PPT 到 PDF 的转换转换。

## 4. 实施过程 (Implementation Progress)

### 4.1 接口定义与客户端实现 (Completed)
*   **Protobuf 定义**: `api/proto/mineru/mineru.proto` 声明了高精度解析接口。
*   **Go Client**: `pkg/mineru/client.go` 封装了 gRPC 调用逻辑，支持超时与连接池。

### 4.2 数据工程管线落地 (Completed)
*   **模型层**: `internal/model/structured_data.go` 新增了 MySQL JSON 存储模型。
*   **逻辑层**: `internal/service/excel_service.go` 利用 `excelize` 实现 Excel 到 JSON 的智能化映射（自动提取表头）。
*   **持久层**: `internal/repository/structured_data_repository.go` 支持批量分块写入 MySQL。

### 4.3 核心流水线重构 (Completed)
*   **Processor 升级**: `internal/pipeline/processor.go` 已实现智能路由：
    - `.pdf`/`.docx`/`.pptx` -> **MinerU** (Markdown 提取)
    - `.xlsx`/`.csv` -> **ExcelService** (数据工程导入)
    - `.txt`/`.md` -> **Native** (直接读取)
*   **依赖注入卸载**: `cmd/server/main.go` 与 `rag-admin.go` 已彻底移除 `tika` 相关代码，改用 `mineruClient` 和 `excelService`。

### 4.4 预览服务适配 (Completed)
*   **DocumentService**: `GetFilePreviewContent` 已完成适配。现在预览 PDF 或 Word 文档时，将直接展示由 MinerU 解析出的精美 Markdown 内容，而非 Tika 的杂乱纯文本。

## 5. 预期收益
*   **检索准确度提升**：MinerU 的版面分析能力可将 PDF 检索精度提升约 40%-60%。
*   **计算能力增强**：通过 Text-to-SQL，Agent 将具备对 Excel 进行求和、平均、分类汇总等计算能力。
*   **系统轻量化**：已为后续彻底移除 `apache/tika` 容器做好准备。

---
*文档版本：v1.1 (全链路 Go 集成完成)*
*更新日期：2026-03-16*
