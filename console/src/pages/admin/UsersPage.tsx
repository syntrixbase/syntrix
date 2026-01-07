import { useState, useEffect, useCallback } from 'react'
import { adminApi } from '../../lib/admin'
import type { User } from '../../lib/admin'
import { UserList } from '../../components/features/users'
import { Spinner } from '../../components/ui/Loading'
import { useToast } from '../../components/ui/Toast'
import { Input } from '../../components/ui/Input'
import { Button } from '../../components/ui/Button'
import { Users, Search, RefreshCw } from 'lucide-react'

export default function UsersPage() {
  const [users, setUsers] = useState<User[]>([])
  const [filteredUsers, setFilteredUsers] = useState<User[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState<'all' | 'active' | 'disabled'>('all')
  const toast = useToast()

  const fetchUsers = useCallback(async () => {
    setIsLoading(true)
    try {
      const data = await adminApi.listUsers(100, 0)
      setUsers(data)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to load users')
    } finally {
      setIsLoading(false)
    }
  }, [toast])

  useEffect(() => {
    fetchUsers()
  }, [fetchUsers])

  // Filter users based on search and status
  useEffect(() => {
    let result = users

    // Search filter
    if (searchQuery.trim()) {
      const query = searchQuery.toLowerCase()
      result = result.filter(user => 
        user.username.toLowerCase().includes(query) ||
        user.roles.some(role => role.toLowerCase().includes(query))
      )
    }

    // Status filter
    if (statusFilter === 'active') {
      result = result.filter(user => !user.disabled)
    } else if (statusFilter === 'disabled') {
      result = result.filter(user => user.disabled)
    }

    setFilteredUsers(result)
  }, [users, searchQuery, statusFilter])

  const handleError = (message: string) => {
    toast.error(message)
  }

  const handleSuccess = (message: string) => {
    toast.success(message)
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="p-2 bg-purple-100 dark:bg-purple-900/30 rounded-lg">
            <Users className="w-6 h-6 text-purple-600 dark:text-purple-400" />
          </div>
          <div>
            <h1 className="text-2xl font-bold text-gray-900 dark:text-white">User Management</h1>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              {users.length} users total
            </p>
          </div>
        </div>
        <Button
          variant="secondary"
          onClick={fetchUsers}
          disabled={isLoading}
        >
          <RefreshCw className={`w-4 h-4 mr-2 ${isLoading ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {/* Filters */}
      <div className="flex flex-col sm:flex-row gap-4">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
          <Input
            placeholder="Search by username or role..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-10"
          />
        </div>
        <div className="flex gap-2">
          {(['all', 'active', 'disabled'] as const).map((status) => (
            <button
              key={status}
              onClick={() => setStatusFilter(status)}
              className={`px-4 py-2 rounded-lg text-sm font-medium transition-colors ${
                statusFilter === status
                  ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
                  : 'bg-gray-100 text-gray-600 dark:bg-gray-800 dark:text-gray-400 hover:bg-gray-200 dark:hover:bg-gray-700'
              }`}
            >
              {status.charAt(0).toUpperCase() + status.slice(1)}
            </button>
          ))}
        </div>
      </div>

      {/* User List */}
      <div className="bg-white dark:bg-gray-900 rounded-xl border border-gray-200 dark:border-gray-800 overflow-hidden">
        {isLoading ? (
          <div className="p-8 flex justify-center">
            <Spinner size="lg" />
          </div>
        ) : filteredUsers.length === 0 ? (
          <div className="p-8 text-center text-gray-500 dark:text-gray-400">
            {searchQuery || statusFilter !== 'all' 
              ? 'No users match your filters'
              : 'No users found'}
          </div>
        ) : (
          <UserList
            users={filteredUsers}
            onUserUpdated={fetchUsers}
            onError={handleError}
            onSuccess={handleSuccess}
          />
        )}
      </div>
    </div>
  )
}
