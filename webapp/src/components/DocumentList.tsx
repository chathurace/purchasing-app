import { useRef } from "react";
import { Box, Button, Card, CardContent, Divider, Link as MuiLink, Stack, Typography } from "@wso2/oxygen-ui";
import { Plus } from "@wso2/oxygen-ui-icons-react";
import type { Document } from "../types/api";

interface Props {
  documents: Document[];
  canEdit: boolean;
  accept?: string; // e.g. ".pdf,.docx"
  title?: string;
  onUpload: (file: File) => void;
  onDelete: (docId: number) => void;
  onDownload: (doc: Document) => void;
  onError?: (message: string) => void;
}

// extPattern turns an accept string like ".pdf,.docx" into a validation regex.
function extPattern(accept: string): RegExp {
  const exts = accept
    .split(",")
    .map((e) => e.trim().replace(/^\./, ""))
    .filter(Boolean)
    .join("|");
  return new RegExp(`\\.(${exts})$`, "i");
}

export function DocumentList({
  documents,
  canEdit,
  accept = ".pdf,.docx",
  title = "Documents",
  onUpload,
  onDelete,
  onDownload,
  onError,
}: Props) {
  const fileRef = useRef<HTMLInputElement>(null);
  const pattern = extPattern(accept);

  const onPick = (file: File | null) => {
    if (!file) return;
    if (!pattern.test(file.name)) {
      onError?.(`Only ${accept} files are allowed (got ${file.name}).`);
      if (fileRef.current) fileRef.current.value = "";
      return;
    }
    onUpload(file);
    if (fileRef.current) fileRef.current.value = "";
  };

  return (
    <Card variant="outlined">
      <CardContent>
        <Box sx={{ mb: 1.5, display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <Typography variant="h6" sx={{ fontWeight: 600 }}>
            {title}
          </Typography>
          {canEdit && (
            <Button component="label" variant="text" size="small" startIcon={<Plus size={16} />}>
              Add document
              <input
                ref={fileRef}
                type="file"
                accept={accept}
                hidden
                onChange={(e) => onPick(e.target.files?.[0] ?? null)}
              />
            </Button>
          )}
        </Box>
        {documents.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No documents attached.
          </Typography>
        ) : (
          <Stack divider={<Divider />}>
            {documents.map((doc) => (
              <Box
                key={doc.id}
                sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", py: 1 }}
              >
                <MuiLink
                  component="button"
                  type="button"
                  variant="body2"
                  onClick={() => onDownload(doc)}
                  sx={{ textAlign: "left" }}
                >
                  {doc.filename}
                </MuiLink>
                <Stack direction="row" spacing={1.5} alignItems="center">
                  <Typography variant="caption" color="text.secondary">
                    {(doc.size_bytes / 1024).toFixed(0)} KB
                  </Typography>
                  {canEdit && (
                    <Button variant="text" size="small" color="error" onClick={() => onDelete(doc.id)}>
                      Remove
                    </Button>
                  )}
                </Stack>
              </Box>
            ))}
          </Stack>
        )}
      </CardContent>
    </Card>
  );
}
