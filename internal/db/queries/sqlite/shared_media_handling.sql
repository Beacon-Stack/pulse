-- name: GetSharedMediaHandling :one
SELECT * FROM shared_media_handling WHERE id = 1;

-- name: UpdateSharedMediaHandling :one
UPDATE shared_media_handling SET
    colon_replacement     = ?,
    import_extra_files    = ?,
    extra_file_extensions = ?,
    rename_files          = ?,
    updated_at            = ?
WHERE id = 1
RETURNING *;
