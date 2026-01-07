import { useState, useEffect } from 'react';
import { Database, ChevronRight, FolderOpen, RefreshCw } from 'lucide-react';
import { documentsApi, type CollectionInfo } from '../../../lib/documents';
import { Spinner } from '../../ui';

interface CollectionTreeProps {
  selectedCollection: string | null;
  onSelectCollection: (name: string) => void;
}

export function CollectionTree({ selectedCollection, onSelectCollection }: CollectionTreeProps) {
  const [collections, setCollections] = useState<CollectionInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchCollections = async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await documentsApi.listCollections();
      setCollections(data);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load collections');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCollections();
  }, []);

  return (
    <div className="w-64 border-r border-gray-200 dark:border-gray-700 bg-gray-50 dark:bg-gray-800/50 flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200 dark:border-gray-700">
        <div className="flex items-center gap-2">
          <Database className="w-4 h-4 text-gray-500" />
          <span className="text-sm font-medium text-gray-700 dark:text-gray-300">Collections</span>
        </div>
        <button
          onClick={fetchCollections}
          disabled={loading}
          className="p-1 rounded hover:bg-gray-200 dark:hover:bg-gray-700 text-gray-500 disabled:opacity-50"
          title="Refresh collections"
        >
          <RefreshCw className={`w-4 h-4 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {/* Collection List */}
      <div className="flex-1 overflow-y-auto py-2">
        {loading && collections.length === 0 ? (
          <div className="flex items-center justify-center py-8">
            <Spinner size="sm" />
          </div>
        ) : error ? (
          <div className="px-4 py-2 text-sm text-red-600 dark:text-red-400">{error}</div>
        ) : collections.length === 0 ? (
          <div className="px-4 py-2 text-sm text-gray-500">No collections found</div>
        ) : (
          <ul className="space-y-0.5">
            {collections.map((collection) => {
              const isSelected = selectedCollection === collection.name;
              return (
                <li key={collection.name}>
                  <button
                    onClick={() => onSelectCollection(collection.name)}
                    className={`
                      w-full flex items-center gap-2 px-4 py-2 text-sm text-left transition-colors
                      ${isSelected 
                        ? 'bg-blue-100 dark:bg-blue-900/30 text-blue-700 dark:text-blue-300' 
                        : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700/50'
                      }
                    `}
                  >
                    {isSelected ? (
                      <FolderOpen className="w-4 h-4 flex-shrink-0" />
                    ) : (
                      <ChevronRight className="w-4 h-4 flex-shrink-0" />
                    )}
                    <span className="truncate">{collection.name}</span>
                    {collection.count !== undefined && (
                      <span className="ml-auto text-xs text-gray-400">{collection.count}</span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </div>
    </div>
  );
}
