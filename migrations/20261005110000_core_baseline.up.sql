-- 由 ad6b259 的 AutoMigrate 在隔離 PostgreSQL 18 重建，固定為 11400 前核心基線。
-- 本檔只用於全新資料庫；既有資料庫須先驗證結構再接管。
--
-- PostgreSQL database dump
--


-- Dumped from database version 18.4 (Debian 18.4-1.pgdg13+1)
-- Dumped by pg_dump version 18.4 (Debian 18.4-1.pgdg13+1)




--
-- Name: ai_detection_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_detection_events (
    id bigint NOT NULL,
    chat_id bigint NOT NULL,
    update_id bigint NOT NULL,
    message_id bigint NOT NULL,
    user_id bigint NOT NULL,
    content_fingerprint text NOT NULL,
    provider character varying(64) NOT NULL,
    model character varying(200) NOT NULL,
    prompt_version character varying(64) NOT NULL,
    schema_version character varying(64),
    rule_version character varying(64),
    status character varying(32) NOT NULL,
    label character varying(32),
    category character varying(100),
    confidence numeric,
    confidence_source character varying(32),
    reason_code character varying(100),
    evidence jsonb,
    safe_action character varying(32),
    error_code character varying(64),
    error_text character varying(500),
    retryable boolean,
    created_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone
);


--
-- Name: TABLE ai_detection_events; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.ai_detection_events IS 'AI 垃圾訊息判定與錯誤稽核紀錄';


--
-- Name: COLUMN ai_detection_events.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.id IS 'AI 判定流水號';


--
-- Name: COLUMN ai_detection_events.chat_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.chat_id IS 'Telegram 聊天識別碼';


--
-- Name: COLUMN ai_detection_events.update_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.update_id IS 'Telegram 更新識別碼';


--
-- Name: COLUMN ai_detection_events.message_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.message_id IS 'Telegram 訊息識別碼';


--
-- Name: COLUMN ai_detection_events.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.user_id IS 'Telegram 成員識別碼';


--
-- Name: COLUMN ai_detection_events.content_fingerprint; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.content_fingerprint IS '有金鑰的內容指紋';


--
-- Name: COLUMN ai_detection_events.provider; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.provider IS 'AI provider 名稱';


--
-- Name: COLUMN ai_detection_events.model; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.model IS 'AI 模型名稱';


--
-- Name: COLUMN ai_detection_events.prompt_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.prompt_version IS 'Prompt 版本';


--
-- Name: COLUMN ai_detection_events.schema_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.schema_version IS 'AI 回應 schema 版本';


--
-- Name: COLUMN ai_detection_events.rule_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.rule_version IS '規則快照版本';


--
-- Name: COLUMN ai_detection_events.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.status IS 'AI 判定狀態';


--
-- Name: COLUMN ai_detection_events.label; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.label IS 'AI 判定標籤';


--
-- Name: COLUMN ai_detection_events.category; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.category IS 'AI 判定分類';


--
-- Name: COLUMN ai_detection_events.confidence; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.confidence IS 'AI 信心分數';


--
-- Name: COLUMN ai_detection_events.confidence_source; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.confidence_source IS 'AI 信心分數來源';


--
-- Name: COLUMN ai_detection_events.reason_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.reason_code IS 'AI 判定原因代碼';


--
-- Name: COLUMN ai_detection_events.evidence; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.evidence IS 'AI 證據摘要，不含完整原文';


--
-- Name: COLUMN ai_detection_events.safe_action; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.safe_action IS 'AI 建議最高安全動作';


--
-- Name: COLUMN ai_detection_events.error_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.error_code IS '穩定錯誤類型';


--
-- Name: COLUMN ai_detection_events.error_text; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.error_text IS '遮罩後錯誤摘要';


--
-- Name: COLUMN ai_detection_events.retryable; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.retryable IS '失敗是否可重試';


--
-- Name: COLUMN ai_detection_events.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.created_at IS '建立 UTC 時間';


--
-- Name: COLUMN ai_detection_events.completed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.ai_detection_events.completed_at IS '完成 UTC 時間';


--
-- Name: ai_detection_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.ai_detection_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ai_detection_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.ai_detection_events_id_seq OWNED BY public.ai_detection_events.id;


--
-- Name: auto_reply_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auto_reply_executions (
    id bigint NOT NULL,
    chat_id bigint NOT NULL,
    update_id bigint NOT NULL,
    message_id bigint NOT NULL,
    user_id bigint NOT NULL,
    rule_id character varying(100) NOT NULL,
    status text NOT NULL,
    error_code character varying(64),
    error_text character varying(500),
    retryable boolean,
    created_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone
);


--
-- Name: TABLE auto_reply_executions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.auto_reply_executions IS 'Telegram 自動回覆執行與稽核紀錄';


--
-- Name: COLUMN auto_reply_executions.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.id IS '自動回覆流水號';


--
-- Name: COLUMN auto_reply_executions.chat_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.chat_id IS 'Telegram 聊天識別碼';


--
-- Name: COLUMN auto_reply_executions.update_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.update_id IS 'Telegram 更新識別碼';


--
-- Name: COLUMN auto_reply_executions.message_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.message_id IS '觸發自動回覆的訊息識別碼';


--
-- Name: COLUMN auto_reply_executions.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.user_id IS '觸發自動回覆的成員識別碼';


--
-- Name: COLUMN auto_reply_executions.rule_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.rule_id IS '命中的自動回覆規則識別碼';


--
-- Name: COLUMN auto_reply_executions.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.status IS '執行狀態';


--
-- Name: COLUMN auto_reply_executions.error_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.error_code IS '穩定錯誤類型';


--
-- Name: COLUMN auto_reply_executions.error_text; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.error_text IS '遮罩後錯誤摘要';


--
-- Name: COLUMN auto_reply_executions.retryable; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.retryable IS '失敗是否可重試';


--
-- Name: COLUMN auto_reply_executions.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.created_at IS '建立 UTC 時間';


--
-- Name: COLUMN auto_reply_executions.completed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.auto_reply_executions.completed_at IS '完成 UTC 時間';


--
-- Name: auto_reply_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.auto_reply_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: auto_reply_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.auto_reply_executions_id_seq OWNED BY public.auto_reply_executions.id;


--
-- Name: command_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.command_executions (
    id bigint NOT NULL,
    chat_id bigint NOT NULL,
    update_id bigint NOT NULL,
    message_id bigint NOT NULL,
    command character varying(32) NOT NULL,
    operator_id bigint NOT NULL,
    target_user_id bigint,
    target_message_id bigint,
    argument_summary character varying(200),
    source character varying(32) NOT NULL,
    status text NOT NULL,
    result character varying(500),
    error_text character varying(500),
    error_code character varying(64),
    retryable boolean,
    created_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone
);


--
-- Name: TABLE command_executions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.command_executions IS 'Telegram 人工管理指令與稽核紀錄';


--
-- Name: COLUMN command_executions.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.id IS '管理指令流水號';


--
-- Name: COLUMN command_executions.chat_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.chat_id IS 'Telegram 聊天識別碼';


--
-- Name: COLUMN command_executions.update_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.update_id IS 'Telegram 更新識別碼';


--
-- Name: COLUMN command_executions.message_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.message_id IS 'Telegram 指令訊息識別碼';


--
-- Name: COLUMN command_executions.command; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.command IS '管理指令名稱';


--
-- Name: COLUMN command_executions.operator_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.operator_id IS '指令操作者識別碼';


--
-- Name: COLUMN command_executions.target_user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.target_user_id IS '目標成員識別碼';


--
-- Name: COLUMN command_executions.target_message_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.target_message_id IS '目標訊息識別碼';


--
-- Name: COLUMN command_executions.argument_summary; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.argument_summary IS '不含秘密值的參數摘要';


--
-- Name: COLUMN command_executions.source; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.source IS '操作來源';


--
-- Name: COLUMN command_executions.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.status IS '執行狀態';


--
-- Name: COLUMN command_executions.result; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.result IS '安全結果摘要';


--
-- Name: COLUMN command_executions.error_text; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.error_text IS '安全錯誤摘要';


--
-- Name: COLUMN command_executions.error_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.error_code IS '穩定錯誤類型';


--
-- Name: COLUMN command_executions.retryable; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.retryable IS '失敗指令是否可重試';


--
-- Name: COLUMN command_executions.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.created_at IS '建立 UTC 時間';


--
-- Name: COLUMN command_executions.completed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.command_executions.completed_at IS '完成 UTC 時間';


--
-- Name: command_executions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.command_executions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: command_executions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.command_executions_id_seq OWNED BY public.command_executions.id;


--
-- Name: detection_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.detection_events (
    event_id text NOT NULL,
    update_id bigint NOT NULL,
    chat_id bigint NOT NULL,
    message_id bigint NOT NULL,
    user_id bigint NOT NULL,
    content_fingerprint text NOT NULL,
    category_id text,
    severity text,
    rule_version text,
    mode text,
    score bigint,
    threshold bigint,
    is_spam boolean,
    matches jsonb,
    signals jsonb,
    created_at timestamp with time zone NOT NULL
);


--
-- Name: TABLE detection_events; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.detection_events IS '垃圾訊息偵測事件';


--
-- Name: COLUMN detection_events.event_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.event_id IS '事件唯一識別碼';


--
-- Name: COLUMN detection_events.update_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.update_id IS 'Telegram 更新識別碼';


--
-- Name: COLUMN detection_events.chat_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.chat_id IS 'Telegram 聊天識別碼';


--
-- Name: COLUMN detection_events.message_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.message_id IS 'Telegram 訊息識別碼';


--
-- Name: COLUMN detection_events.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.user_id IS 'Telegram 成員識別碼';


--
-- Name: COLUMN detection_events.content_fingerprint; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.content_fingerprint IS '有金鑰的內容指紋';


--
-- Name: COLUMN detection_events.category_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.category_id IS '命中的違規類型';


--
-- Name: COLUMN detection_events.severity; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.severity IS '違規嚴重度';


--
-- Name: COLUMN detection_events.rule_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.rule_version IS '規則快照版本';


--
-- Name: COLUMN detection_events.mode; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.mode IS '執行模式';


--
-- Name: COLUMN detection_events.score; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.score IS '偵測總分';


--
-- Name: COLUMN detection_events.threshold; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.threshold IS '判定門檻';


--
-- Name: COLUMN detection_events.is_spam; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.is_spam IS '是否判定為垃圾訊息';


--
-- Name: COLUMN detection_events.matches; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.matches IS '命中規則摘要';


--
-- Name: COLUMN detection_events.signals; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.signals IS '命中行為訊號摘要';


--
-- Name: COLUMN detection_events.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.detection_events.created_at IS '事件 UTC 時間';


--
-- Name: enforcement_actions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enforcement_actions (
    action_key text NOT NULL,
    event_id text NOT NULL,
    kind text,
    status text,
    retryable boolean,
    error_code text,
    error_text text,
    ended_at timestamp with time zone
);


--
-- Name: TABLE enforcement_actions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.enforcement_actions IS 'Telegram 處置執行紀錄';


--
-- Name: COLUMN enforcement_actions.action_key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.action_key IS '冪等處置鍵';


--
-- Name: COLUMN enforcement_actions.event_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.event_id IS '偵測事件識別碼';


--
-- Name: COLUMN enforcement_actions.kind; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.kind IS '處置種類';


--
-- Name: COLUMN enforcement_actions.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.status IS '處置狀態';


--
-- Name: COLUMN enforcement_actions.retryable; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.retryable IS '是否允許重試';


--
-- Name: COLUMN enforcement_actions.error_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.error_code IS '外部錯誤代碼';


--
-- Name: COLUMN enforcement_actions.error_text; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.error_text IS '遮罩後錯誤摘要';


--
-- Name: COLUMN enforcement_actions.ended_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enforcement_actions.ended_at IS '處置結束 UTC 時間';


--
-- Name: processed_updates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.processed_updates (
    update_id bigint NOT NULL,
    status text NOT NULL,
    claimed_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone
);


--
-- Name: TABLE processed_updates; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.processed_updates IS 'Telegram 更新冪等紀錄';


--
-- Name: COLUMN processed_updates.update_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.processed_updates.update_id IS 'Telegram 更新唯一識別碼';


--
-- Name: COLUMN processed_updates.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.processed_updates.status IS '處理狀態';


--
-- Name: COLUMN processed_updates.claimed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.processed_updates.claimed_at IS '開始處理 UTC 時間';


--
-- Name: COLUMN processed_updates.completed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.processed_updates.completed_at IS '完成處理 UTC 時間';


--
-- Name: processed_updates_update_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.processed_updates_update_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: processed_updates_update_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.processed_updates_update_id_seq OWNED BY public.processed_updates.update_id;


--
-- Name: semantic_manual_samples; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.semantic_manual_samples (
    id bigint NOT NULL,
    chat_id bigint NOT NULL,
    message_id bigint NOT NULL,
    target_user_id bigint NOT NULL,
    operator_id bigint NOT NULL,
    content_fingerprint text NOT NULL,
    label character varying(32) NOT NULL,
    category character varying(100) NOT NULL,
    source character varying(32) NOT NULL,
    status character varying(32) NOT NULL,
    error_code character varying(64),
    error_text character varying(500),
    retryable boolean,
    created_at timestamp with time zone NOT NULL,
    embedded_at timestamp with time zone
);


--
-- Name: TABLE semantic_manual_samples; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.semantic_manual_samples IS '管理員提交的語意垃圾樣本摘要';


--
-- Name: COLUMN semantic_manual_samples.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.id IS '人工語意樣本流水號';


--
-- Name: COLUMN semantic_manual_samples.chat_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.chat_id IS 'Telegram 聊天識別碼';


--
-- Name: COLUMN semantic_manual_samples.message_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.message_id IS 'Telegram 目標訊息識別碼';


--
-- Name: COLUMN semantic_manual_samples.target_user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.target_user_id IS '目標成員識別碼';


--
-- Name: COLUMN semantic_manual_samples.operator_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.operator_id IS '提交樣本的管理員識別碼';


--
-- Name: COLUMN semantic_manual_samples.content_fingerprint; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.content_fingerprint IS '有金鑰的內容指紋';


--
-- Name: COLUMN semantic_manual_samples.label; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.label IS '人工標籤';


--
-- Name: COLUMN semantic_manual_samples.category; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.category IS '人工分類';


--
-- Name: COLUMN semantic_manual_samples.source; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.source IS '樣本來源';


--
-- Name: COLUMN semantic_manual_samples.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.status IS '向量化狀態';


--
-- Name: COLUMN semantic_manual_samples.error_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.error_code IS '穩定錯誤類型';


--
-- Name: COLUMN semantic_manual_samples.error_text; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.error_text IS '遮罩後錯誤摘要';


--
-- Name: COLUMN semantic_manual_samples.retryable; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.retryable IS '失敗是否可重試';


--
-- Name: COLUMN semantic_manual_samples.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.created_at IS '建立 UTC 時間';


--
-- Name: COLUMN semantic_manual_samples.embedded_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_manual_samples.embedded_at IS '向量化完成 UTC 時間';


--
-- Name: semantic_manual_samples_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.semantic_manual_samples_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: semantic_manual_samples_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.semantic_manual_samples_id_seq OWNED BY public.semantic_manual_samples.id;


--
-- Name: trusted_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.trusted_members (
    chat_id bigint NOT NULL,
    user_id bigint NOT NULL,
    reason text,
    enabled boolean,
    created_at timestamp with time zone
);


--
-- Name: TABLE trusted_members; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.trusted_members IS '可信任成員名單';


--
-- Name: COLUMN trusted_members.chat_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.trusted_members.chat_id IS 'Telegram 聊天識別碼';


--
-- Name: COLUMN trusted_members.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.trusted_members.user_id IS '可信任成員識別碼';


--
-- Name: COLUMN trusted_members.reason; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.trusted_members.reason IS '信任原因';


--
-- Name: COLUMN trusted_members.enabled; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.trusted_members.enabled IS '是否啟用豁免';


--
-- Name: COLUMN trusted_members.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.trusted_members.created_at IS '建立 UTC 時間';


--
-- Name: violations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.violations (
    id bigint NOT NULL,
    event_id text NOT NULL,
    chat_id bigint,
    user_id bigint,
    category_id text,
    severity text,
    source text DEFAULT 'auto'::text NOT NULL,
    operator_id bigint,
    reason character varying(200),
    occurred_at timestamp with time zone NOT NULL,
    invalidated_at timestamp with time zone,
    invalidated_by bigint,
    invalidation_reason character varying(200)
);


--
-- Name: TABLE violations; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.violations IS '成員違規紀錄';


--
-- Name: COLUMN violations.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.id IS '違規流水號';


--
-- Name: COLUMN violations.event_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.event_id IS '偵測或人工事件識別碼';


--
-- Name: COLUMN violations.chat_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.chat_id IS 'Telegram 聊天識別碼';


--
-- Name: COLUMN violations.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.user_id IS 'Telegram 成員識別碼';


--
-- Name: COLUMN violations.category_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.category_id IS '違規類型';


--
-- Name: COLUMN violations.severity; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.severity IS '違規嚴重度';


--
-- Name: COLUMN violations.source; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.source IS '自動或人工來源';


--
-- Name: COLUMN violations.operator_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.operator_id IS '人工操作管理員識別碼';


--
-- Name: COLUMN violations.reason; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.reason IS '人工操作原因';


--
-- Name: COLUMN violations.occurred_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.occurred_at IS '違規 UTC 時間';


--
-- Name: COLUMN violations.invalidated_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.invalidated_at IS '違規失效 UTC 時間';


--
-- Name: COLUMN violations.invalidated_by; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.invalidated_by IS '執行失效的管理員識別碼';


--
-- Name: COLUMN violations.invalidation_reason; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.violations.invalidation_reason IS '違規失效原因';


--
-- Name: violations_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.violations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: violations_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.violations_id_seq OWNED BY public.violations.id;


--
-- Name: ai_detection_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_detection_events ALTER COLUMN id SET DEFAULT nextval('public.ai_detection_events_id_seq'::regclass);


--
-- Name: auto_reply_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auto_reply_executions ALTER COLUMN id SET DEFAULT nextval('public.auto_reply_executions_id_seq'::regclass);


--
-- Name: command_executions id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_executions ALTER COLUMN id SET DEFAULT nextval('public.command_executions_id_seq'::regclass);


--
-- Name: processed_updates update_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.processed_updates ALTER COLUMN update_id SET DEFAULT nextval('public.processed_updates_update_id_seq'::regclass);


--
-- Name: semantic_manual_samples id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.semantic_manual_samples ALTER COLUMN id SET DEFAULT nextval('public.semantic_manual_samples_id_seq'::regclass);


--
-- Name: violations id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.violations ALTER COLUMN id SET DEFAULT nextval('public.violations_id_seq'::regclass);


--
-- Name: ai_detection_events ai_detection_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_detection_events
    ADD CONSTRAINT ai_detection_events_pkey PRIMARY KEY (id);


--
-- Name: auto_reply_executions auto_reply_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auto_reply_executions
    ADD CONSTRAINT auto_reply_executions_pkey PRIMARY KEY (id);


--
-- Name: command_executions command_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.command_executions
    ADD CONSTRAINT command_executions_pkey PRIMARY KEY (id);


--
-- Name: detection_events detection_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.detection_events
    ADD CONSTRAINT detection_events_pkey PRIMARY KEY (event_id);


--
-- Name: enforcement_actions enforcement_actions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enforcement_actions
    ADD CONSTRAINT enforcement_actions_pkey PRIMARY KEY (action_key);


--
-- Name: processed_updates processed_updates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.processed_updates
    ADD CONSTRAINT processed_updates_pkey PRIMARY KEY (update_id);


--
-- Name: semantic_manual_samples semantic_manual_samples_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.semantic_manual_samples
    ADD CONSTRAINT semantic_manual_samples_pkey PRIMARY KEY (id);


--
-- Name: trusted_members trusted_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.trusted_members
    ADD CONSTRAINT trusted_members_pkey PRIMARY KEY (chat_id, user_id);


--
-- Name: violations violations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.violations
    ADD CONSTRAINT violations_pkey PRIMARY KEY (id);


--
-- Name: idx_ai_detection_cache; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_detection_cache ON public.ai_detection_events USING btree (content_fingerprint, provider, model, prompt_version, rule_version, created_at);


--
-- Name: idx_ai_detection_events_chat_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_detection_events_chat_id ON public.ai_detection_events USING btree (chat_id);


--
-- Name: idx_ai_detection_events_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_detection_events_created_at ON public.ai_detection_events USING btree (created_at);


--
-- Name: idx_ai_detection_events_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_detection_events_status ON public.ai_detection_events USING btree (status);


--
-- Name: idx_ai_detection_events_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ai_detection_events_user_id ON public.ai_detection_events USING btree (user_id);


--
-- Name: idx_ai_detection_update; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_ai_detection_update ON public.ai_detection_events USING btree (chat_id, update_id);


--
-- Name: idx_auto_reply_executions_rule_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auto_reply_executions_rule_id ON public.auto_reply_executions USING btree (rule_id);


--
-- Name: idx_auto_reply_executions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auto_reply_executions_status ON public.auto_reply_executions USING btree (status);


--
-- Name: idx_auto_reply_executions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_auto_reply_executions_user_id ON public.auto_reply_executions USING btree (user_id);


--
-- Name: idx_auto_reply_update; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_auto_reply_update ON public.auto_reply_executions USING btree (chat_id, update_id);


--
-- Name: idx_command_executions_operator_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_command_executions_operator_id ON public.command_executions USING btree (operator_id);


--
-- Name: idx_command_executions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_command_executions_status ON public.command_executions USING btree (status);


--
-- Name: idx_command_executions_target_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_command_executions_target_user_id ON public.command_executions USING btree (target_user_id);


--
-- Name: idx_command_update; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_command_update ON public.command_executions USING btree (chat_id, update_id);


--
-- Name: idx_detection_events_chat_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_detection_events_chat_id ON public.detection_events USING btree (chat_id);


--
-- Name: idx_detection_events_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_detection_events_created_at ON public.detection_events USING btree (created_at);


--
-- Name: idx_detection_events_update_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_detection_events_update_id ON public.detection_events USING btree (update_id);


--
-- Name: idx_detection_events_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_detection_events_user_id ON public.detection_events USING btree (user_id);


--
-- Name: idx_enforcement_actions_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_enforcement_actions_event_id ON public.enforcement_actions USING btree (event_id);


--
-- Name: idx_manual_sample_message; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_manual_sample_message ON public.semantic_manual_samples USING btree (chat_id, message_id);


--
-- Name: idx_semantic_manual_samples_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_category ON public.semantic_manual_samples USING btree (category);


--
-- Name: idx_semantic_manual_samples_chat_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_chat_id ON public.semantic_manual_samples USING btree (chat_id);


--
-- Name: idx_semantic_manual_samples_content_fingerprint; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_content_fingerprint ON public.semantic_manual_samples USING btree (content_fingerprint);


--
-- Name: idx_semantic_manual_samples_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_created_at ON public.semantic_manual_samples USING btree (created_at);


--
-- Name: idx_semantic_manual_samples_label; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_label ON public.semantic_manual_samples USING btree (label);


--
-- Name: idx_semantic_manual_samples_operator_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_operator_id ON public.semantic_manual_samples USING btree (operator_id);


--
-- Name: idx_semantic_manual_samples_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_status ON public.semantic_manual_samples USING btree (status);


--
-- Name: idx_semantic_manual_samples_target_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_manual_samples_target_user_id ON public.semantic_manual_samples USING btree (target_user_id);


--
-- Name: idx_violation_member_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_violation_member_time ON public.violations USING btree (chat_id, user_id, occurred_at);


--
-- Name: idx_violations_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_violations_event_id ON public.violations USING btree (event_id);


--
-- Name: idx_violations_invalidated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_violations_invalidated_at ON public.violations USING btree (invalidated_at);


--
-- Name: idx_violations_source; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_violations_source ON public.violations USING btree (source);


--
-- PostgreSQL database dump complete
--
