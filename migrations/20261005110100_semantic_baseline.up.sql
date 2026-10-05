-- 由 ad6b259 的語意 AutoMigrate 在隔離 PostgreSQL 18 + pgvector 0.8.5 重建。
-- 執行前必須由部署角色明示安裝 vector extension；語意功能關閉時不得套用。
--
-- PostgreSQL database dump
--


-- Dumped from database version 18.4 (Debian 18.4-1.pgdg12+1)
-- Dumped by pg_dump version 18.4 (Debian 18.4-1.pgdg12+1)




--
-- Name: message_embeddings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.message_embeddings (
    id bigint NOT NULL,
    content_fingerprint text NOT NULL,
    embedding_provider character varying(64) NOT NULL,
    embedding_model character varying(200) NOT NULL,
    embedding_version character varying(64) NOT NULL,
    dimensions bigint NOT NULL,
    vector public.vector NOT NULL,
    label character varying(32) NOT NULL,
    category character varying(100),
    reason_code character varying(100),
    created_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL
);


--
-- Name: TABLE message_embeddings; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.message_embeddings IS '訊息 embedding 與語意垃圾記憶紀錄';


--
-- Name: COLUMN message_embeddings.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.id IS '訊息向量流水號';


--
-- Name: COLUMN message_embeddings.content_fingerprint; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.content_fingerprint IS '有金鑰的內容指紋';


--
-- Name: COLUMN message_embeddings.embedding_provider; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.embedding_provider IS 'Embedding provider 名稱';


--
-- Name: COLUMN message_embeddings.embedding_model; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.embedding_model IS 'Embedding 模型名稱';


--
-- Name: COLUMN message_embeddings.embedding_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.embedding_version IS 'Embedding 版本';


--
-- Name: COLUMN message_embeddings.dimensions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.dimensions IS '向量維度';


--
-- Name: COLUMN message_embeddings.vector; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.vector IS '訊息 embedding 向量';


--
-- Name: COLUMN message_embeddings.label; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.label IS '語意標籤';


--
-- Name: COLUMN message_embeddings.category; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.category IS '語意分類';


--
-- Name: COLUMN message_embeddings.reason_code; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.reason_code IS '語意原因代碼';


--
-- Name: COLUMN message_embeddings.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.created_at IS '建立 UTC 時間';


--
-- Name: COLUMN message_embeddings.expires_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.message_embeddings.expires_at IS '過期 UTC 時間';


--
-- Name: message_embeddings_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.message_embeddings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: message_embeddings_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.message_embeddings_id_seq OWNED BY public.message_embeddings.id;


--
-- Name: semantic_blacklist_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.semantic_blacklist_categories (
    id character varying(100) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500),
    enabled boolean NOT NULL,
    created_at timestamp with time zone NOT NULL
);


--
-- Name: TABLE semantic_blacklist_categories; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.semantic_blacklist_categories IS '語意黑名單分類';


--
-- Name: COLUMN semantic_blacklist_categories.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_categories.id IS '語意黑名單分類識別碼';


--
-- Name: COLUMN semantic_blacklist_categories.name; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_categories.name IS '分類名稱';


--
-- Name: COLUMN semantic_blacklist_categories.description; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_categories.description IS '分類說明';


--
-- Name: COLUMN semantic_blacklist_categories.enabled; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_categories.enabled IS '是否啟用';


--
-- Name: COLUMN semantic_blacklist_categories.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_categories.created_at IS '建立 UTC 時間';


--
-- Name: semantic_blacklist_examples; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.semantic_blacklist_examples (
    id bigint NOT NULL,
    category_id character varying(100) NOT NULL,
    embedding_provider character varying(64) NOT NULL,
    embedding_model character varying(200) NOT NULL,
    embedding_version character varying(64) NOT NULL,
    dimensions bigint NOT NULL,
    vector public.vector NOT NULL,
    source character varying(32) NOT NULL,
    created_at timestamp with time zone NOT NULL
);


--
-- Name: TABLE semantic_blacklist_examples; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.semantic_blacklist_examples IS '語意黑名單範例向量';


--
-- Name: COLUMN semantic_blacklist_examples.id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.id IS '語意黑名單範例流水號';


--
-- Name: COLUMN semantic_blacklist_examples.category_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.category_id IS '語意黑名單分類識別碼';


--
-- Name: COLUMN semantic_blacklist_examples.embedding_provider; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.embedding_provider IS 'Embedding provider 名稱';


--
-- Name: COLUMN semantic_blacklist_examples.embedding_model; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.embedding_model IS 'Embedding 模型名稱';


--
-- Name: COLUMN semantic_blacklist_examples.embedding_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.embedding_version IS 'Embedding 版本';


--
-- Name: COLUMN semantic_blacklist_examples.dimensions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.dimensions IS '向量維度';


--
-- Name: COLUMN semantic_blacklist_examples.vector; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.vector IS '黑名單範例 embedding 向量';


--
-- Name: COLUMN semantic_blacklist_examples.source; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.source IS '建立來源';


--
-- Name: COLUMN semantic_blacklist_examples.created_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.semantic_blacklist_examples.created_at IS '建立 UTC 時間';


--
-- Name: semantic_blacklist_examples_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.semantic_blacklist_examples_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: semantic_blacklist_examples_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.semantic_blacklist_examples_id_seq OWNED BY public.semantic_blacklist_examples.id;


--
-- Name: message_embeddings id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_embeddings ALTER COLUMN id SET DEFAULT nextval('public.message_embeddings_id_seq'::regclass);


--
-- Name: semantic_blacklist_examples id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.semantic_blacklist_examples ALTER COLUMN id SET DEFAULT nextval('public.semantic_blacklist_examples_id_seq'::regclass);


--
-- Name: message_embeddings message_embeddings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.message_embeddings
    ADD CONSTRAINT message_embeddings_pkey PRIMARY KEY (id);


--
-- Name: semantic_blacklist_categories semantic_blacklist_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.semantic_blacklist_categories
    ADD CONSTRAINT semantic_blacklist_categories_pkey PRIMARY KEY (id);


--
-- Name: semantic_blacklist_examples semantic_blacklist_examples_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.semantic_blacklist_examples
    ADD CONSTRAINT semantic_blacklist_examples_pkey PRIMARY KEY (id);


--
-- Name: idx_blacklist_example_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blacklist_example_scope ON public.semantic_blacklist_examples USING btree (embedding_provider, embedding_model, embedding_version, dimensions);


--
-- Name: idx_message_embedding_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_embedding_scope ON public.message_embeddings USING btree (embedding_provider, embedding_model, embedding_version, dimensions);


--
-- Name: idx_message_embedding_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_message_embedding_unique ON public.message_embeddings USING btree (content_fingerprint, embedding_provider, embedding_model, embedding_version, dimensions);


--
-- Name: idx_message_embeddings_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_embeddings_category ON public.message_embeddings USING btree (category);


--
-- Name: idx_message_embeddings_content_fingerprint; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_embeddings_content_fingerprint ON public.message_embeddings USING btree (content_fingerprint);


--
-- Name: idx_message_embeddings_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_embeddings_created_at ON public.message_embeddings USING btree (created_at);


--
-- Name: idx_message_embeddings_expires_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_embeddings_expires_at ON public.message_embeddings USING btree (expires_at);


--
-- Name: idx_message_embeddings_label; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_message_embeddings_label ON public.message_embeddings USING btree (label);


--
-- Name: idx_semantic_blacklist_categories_enabled; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_blacklist_categories_enabled ON public.semantic_blacklist_categories USING btree (enabled);


--
-- Name: idx_semantic_blacklist_examples_category_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_semantic_blacklist_examples_category_id ON public.semantic_blacklist_examples USING btree (category_id);


--
-- PostgreSQL database dump complete
--
