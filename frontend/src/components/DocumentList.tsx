import { useRef } from "react";
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
    <div className="rounded border bg-white p-6">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="font-medium text-gray-900">{title}</h2>
        {canEdit && (
          <label className="cursor-pointer text-sm text-indigo-600">
            + Add document
            <input
              ref={fileRef}
              type="file"
              accept={accept}
              className="hidden"
              onChange={(e) => onPick(e.target.files?.[0] ?? null)}
            />
          </label>
        )}
      </div>
      {documents.length === 0 ? (
        <p className="text-sm text-gray-400">No documents attached.</p>
      ) : (
        <ul className="divide-y">
          {documents.map((doc) => (
            <li key={doc.id} className="flex items-center justify-between py-2 text-sm">
              <button className="text-indigo-600 hover:underline" onClick={() => onDownload(doc)}>
                {doc.filename}
              </button>
              <div className="flex items-center gap-3 text-gray-400">
                <span>{(doc.size_bytes / 1024).toFixed(0)} KB</span>
                {canEdit && (
                  <button className="hover:text-red-600" onClick={() => onDelete(doc.id)}>
                    Remove
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
