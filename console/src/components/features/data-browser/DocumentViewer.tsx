import { useState } from 'react';
import { X, Copy, Check, Edit2, Trash2 } from 'lucide-react';
import type { Document } from '../../../lib/documents';
import { Button } from '../../ui';

interface DocumentViewerProps {
  document: Document;
  onClose: () => void;
  onEdit: (doc: Document) => void;
  onDelete: (doc: Document) => void;
}

export function DocumentViewer({ document, onClose, onEdit, onDelete }: DocumentViewerProps) {
  const [copied, setCopied] = useState(false);

  const jsonString = JSON.stringify(document, null, 2);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(jsonString);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  return (
    <div className="w-96 border-l border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900 flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200 dark:border-gray-700">
        <div className="flex-1 min-w-0">
          <h3 className="font-medium text-gray-900 dark:text-white truncate" title={document.id}>
            {document.id}
          </h3>
          {document._collection && (
            <p className="text-xs text-gray-500 truncate">{document._collection}</p>
          )}
        </div>
        <button
          onClick={onClose}
          className="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-500"
        >
          <X className="w-4 h-4" />
        </button>
      </div>

      {/* Actions */}
      <div className="flex items-center gap-2 px-4 py-2 border-b border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50">
        <Button variant="ghost" size="sm" onClick={handleCopy}>
          {copied ? <Check className="w-4 h-4 text-green-500" /> : <Copy className="w-4 h-4" />}
          <span className="ml-1.5">{copied ? 'Copied' : 'Copy'}</span>
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onEdit(document)}>
          <Edit2 className="w-4 h-4" />
          <span className="ml-1.5">Edit</span>
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onDelete(document)} className="text-red-600 hover:text-red-700">
          <Trash2 className="w-4 h-4" />
          <span className="ml-1.5">Delete</span>
        </Button>
      </div>

      {/* JSON Content */}
      <div className="flex-1 overflow-auto p-4">
        <pre className="text-sm font-mono whitespace-pre-wrap break-words text-gray-800 dark:text-gray-200">
          <JsonHighlight json={jsonString} />
        </pre>
      </div>

      {/* Metadata Footer */}
      {(document._createdAt || document._updatedAt) && (
        <div className="px-4 py-2 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 text-xs text-gray-500">
          {document._createdAt && <div>Created: {formatDateTime(document._createdAt)}</div>}
          {document._updatedAt && <div>Updated: {formatDateTime(document._updatedAt)}</div>}
        </div>
      )}
    </div>
  );
}

// Simple JSON syntax highlighting component
function JsonHighlight({ json }: { json: string }) {
  // Basic syntax highlighting using regex
  const highlighted = json
    // String keys
    .replace(/"([^"]+)":/g, '<span class="text-purple-600 dark:text-purple-400">"$1"</span>:')
    // String values
    .replace(/: "([^"]*)"/g, ': <span class="text-green-600 dark:text-green-400">"$1"</span>')
    // Numbers
    .replace(/: (\d+\.?\d*)/g, ': <span class="text-blue-600 dark:text-blue-400">$1</span>')
    // Booleans and null
    .replace(/: (true|false|null)/g, ': <span class="text-orange-600 dark:text-orange-400">$1</span>');

  return <span dangerouslySetInnerHTML={{ __html: highlighted }} />;
}

function formatDateTime(dateStr: string): string {
  try {
    return new Date(dateStr).toLocaleString();
  } catch {
    return dateStr;
  }
}
