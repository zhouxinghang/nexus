-- Server-side file-type classification. resource_kind maps a Resource's name +
-- directory flag to one of the Files taxonomy buckets, mirroring the client's
-- Finder/Drive-style grouping. A generated stored column lets type filtering
-- and per-kind counts run in SQL alongside the existing offset pagination,
-- rather than only over the page the client has loaded.
CREATE FUNCTION resource_kind(name TEXT, is_dir BOOLEAN) RETURNS TEXT
    LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE
        WHEN is_dir THEN 'folder'
        WHEN lower(name) IN ('dockerfile', 'makefile', 'rakefile') THEN 'code'
        WHEN ext = ANY (ARRAY['pdf','doc','docx','txt','md','markdown','rtf','odt','pages','tex','epub']) THEN 'document'
        WHEN ext = ANY (ARRAY['jpg','jpeg','png','gif','webp','svg','bmp','ico','heic','heif','tiff','tif','avif']) THEN 'image'
        WHEN ext = ANY (ARRAY['mp4','mov','avi','mkv','webm','flv','wmv','m4v','mpeg','mpg','3gp']) THEN 'video'
        WHEN ext = ANY (ARRAY['mp3','wav','flac','aac','ogg','oga','m4a','wma','opus','aiff']) THEN 'audio'
        WHEN ext = ANY (ARRAY['xls','xlsx','csv','tsv','ods','numbers']) THEN 'spreadsheet'
        WHEN ext = ANY (ARRAY['ppt','pptx','odp','key']) THEN 'presentation'
        WHEN ext = ANY (ARRAY['js','jsx','ts','tsx','go','py','rb','php','java','kt','c','h','cc','cpp','hpp','cs','rs','swift','sh','bash','zsh','html','htm','css','scss','sass','less','json','yaml','yml','toml','xml','sql','vue','svelte','lua','pl','r','dart','gradle','ini','conf','env']) THEN 'code'
        WHEN ext = ANY (ARRAY['zip','tar','gz','tgz','bz2','xz','rar','7z','zst','lz','lzma']) THEN 'archive'
        ELSE 'other'
    END
    -- The lowercase extension (characters after the final dot), or NULL for a
    -- dotfile like '.env' or an extension-less name, which fall through to the
    -- name/else branches above.
    FROM (SELECT lower((regexp_match(name, '^.+\.([^.]+)$'))[1])) AS e(ext);
$$;

ALTER TABLE resources
    ADD COLUMN kind TEXT NOT NULL GENERATED ALWAYS AS (resource_kind(name, is_dir)) STORED;

CREATE INDEX resources_kind_idx ON resources (kind);
