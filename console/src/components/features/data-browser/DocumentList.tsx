import { useState, useEffect, useCallback } from 'react';
import { ChevronLeft, ChevronRight, RefreshCw } from 'lucide-react';
import { documentsApi, type Document, type QueryResponse } from '../../../lib/documents';
import { Table, type Column, Button, Spinner } from '../../ui';

interface DocumentListProps {
  collection: string;
  selectedDocumentId?: string | null;
  onSelectDocument: (doc: Document) => void;
}

export function DocumentList({ collection, onSelectDocument }: DocumentListProps) {
  const [documents, setDocuments] = useState<Document[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [cursors, setCursors] = useState<string[]>([]); // Stack of cursors for prev navigation
  const [currentCursor, setCurrentCursor] = useState<string | undefined>(undefined);
  const limit = 20;

  const fetchDocuments = useCallback(async (startAfter?: string, isGoingBack = false) => {
    setLoading(true);
    setError(null);
    try {
      const response: QueryResponse = await documentsApi.query({
        collection,
        limit,
        startAfter,
      });
      setDocuments(response.documents);
      setHasMore(response.hasMore);
      setCurrentCursor(startAfter);
      
      // If going forward, save current cursor for back navigation
      if (!isGoingBack && startAfter === undefined) {
        setCursors([]);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load documents');
      setDocuments([]);
    } finally {
      setLoading(false);
    }
  }, [collection]);

  useEffect(() => {
    fetchDocuments();
  }, [collection, fetchDocuments]);

  const handleNextPage = () => {
    if (hasMore && documents.length > 0) {
      const lastDoc = documents[documents.length - 1];
      // Save current cursor before moving forward
      setCursors(prev => [...prev, currentCursor || '']);
      fetchDocuments(lastDoc.id);
    }
  };

  const handlePrevPage = () => {
    if (cursors.length > 0) {
      const newCursors = [...cursors];
      const prevCursor = newCursors.pop();
      setCursors(newCursors);
      fetchDocuments(prevCursor || undefined, true);
    }
  };

  const handleRefresh = () => {
    setCursors([]);
    setCurrentCursor(undefined);
    fetchDocuments();
  };

  const columns: Column<Document>[] = [
    {
      key: 'id',
      header: 'ID',
      width: '200px',
      render: (doc) => (
        <span className="font-mono text-xs truncate block max-w-[180px]" title={doc.id}>
          {doc.id}
        </span>
      ),
    },
    {
      key: 'preview',
      header: 'Preview',
      render: (doc) => {
        const preview = getDocumentPreview(doc);
        return (
          <span className="text-gray-600 dark:text-gray-400 truncate block max-w-[300px]" title={preview}>
            {preview}
          </span>
        );
      },
    },
    {
      key: 'createdAt',
      header: 'Created',
      width: '150px',
      render: (doc) => {
        // Support both createdAt (backend) and _createdAt (legacy)
        const createdAt = doc.createdAt || doc._createdAt;
        return (
          <span className="text-xs text-gray-500">
            {createdAt ? formatDate(createdAt as string | number) : '-'}
          </span>
        );
      },
    },
  ];

  const currentPage = cursors.length + 1;
  const canGoPrev = cursors.length > 0;
  const canGoNext = hasMore;

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center py-12 text-center">
        <p className="text-red-600 dark:text-red-400 mb-4">{error}</p>
        <Button variant="secondary" onClick={handleRefresh}>
          <RefreshCw className="w-4 h-4 mr-2" />
          Retry
        </Button>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200 dark:border-gray-700">
        <div className="flex items-center gap-2">
          <span className="font-medium text-gray-900 dark:text-white">{collection}</span>
          {loading && <Spinner size="sm" />}
        </div>
        <button
          onClick={handleRefresh}
          disabled={loading}
          className="p-1.5 rounded hover:bg-gray-100 dark:hover:bg-gray-700 text-gray-500 disabled:opacity-50"
          title="Refresh"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        <Table
          columns={columns}
          data={documents}
          loading={loading && documents.length === 0}
          emptyMessage={`No documents in ${collection}`}
          onRowClick={onSelectDocument}
          rowKey={(doc) => doc.id}
        />
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-between px-4 py-3 border-t border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50">
        <span className="text-sm text-gray-500">
          {documents.length > 0 
            ? `${documents.length} documents${hasMore ? '+' : ''}`
            : '0 documents'
          }
        </span>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={handlePrevPage}
            disabled={!canGoPrev || loading}
          >
            <ChevronLeft className="w-4 h-4" />
          </Button>
          <span className="text-sm text-gray-600 dark:text-gray-400">
            Page {currentPage}
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={handleNextPage}
            disabled={!canGoNext || loading}
          >
            <ChevronRight className="w-4 h-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

// Helper functions
function getDocumentPreview(doc: Document): string {
  // Priority: text field first, then other fields
  if (doc.text && typeof doc.text === 'string') {
    return doc.text.length > 60 ? doc.text.slice(0, 60) + '...' : doc.text;
  }
  
  const excluded = ['id', 'collection', '_collection', 'createdAt', '_createdAt', 'updatedAt', '_updatedAt', 'version'];
  const entries = Object.entries(doc).filter(([key]) => !excluded.includes(key));
  
  if (entries.length === 0) return '{}';
  
  const [key, value] = entries[0];
  if (typeof value === 'string') {
    return value.length > 50 ? value.slice(0, 50) + '...' : value;
  }
  return `${key}: ${JSON.stringify(value)}`.slice(0, 60);
}

function formatDate(dateValue: string | number): string {
  try {
    // Handle both timestamp (milliseconds) and ISO string
    const date = typeof dateValue === 'number' 
      ? new Date(dateValue) 
      : new Date(dateValue);
    
    if (isNaN(date.getTime())) return '-';
    
    return date.toLocaleDateString('en-US', {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return '-';
  }
}
