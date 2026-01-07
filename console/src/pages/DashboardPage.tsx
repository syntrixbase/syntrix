import { useState, useEffect, useCallback } from 'react'
import { useAuthStore } from '../stores'
import { adminApi } from '../lib/admin'
import { documentsApi } from '../lib/documents'
import { api } from '../lib/api'
import { Spinner } from '../components/ui/Loading'
import { 
  Database, 
  Users, 
  FileText, 
  Zap, 
  Activity, 
  RefreshCw,
  CheckCircle,
  XCircle,
  Clock
} from 'lucide-react'
import { Button } from '../components/ui/Button'

interface DashboardStats {
  collections: number
  documents: number
  users: number
  triggers: number
  healthStatus: 'healthy' | 'unhealthy' | 'unknown'
  lastChecked: Date | null
}

interface StatCardProps {
  title: string
  value: number | string
  icon: React.ReactNode
  loading?: boolean
  color?: string
}

function StatCard({ title, value, icon, loading, color = 'blue' }: StatCardProps) {
  const colorClasses: Record<string, string> = {
    blue: 'bg-blue-100 text-blue-600 dark:bg-blue-900/30 dark:text-blue-400',
    green: 'bg-green-100 text-green-600 dark:bg-green-900/30 dark:text-green-400',
    purple: 'bg-purple-100 text-purple-600 dark:bg-purple-900/30 dark:text-purple-400',
    amber: 'bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400',
  }

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium text-gray-500 dark:text-gray-400">{title}</p>
          {loading ? (
            <div className="mt-2 h-9 flex items-center">
              <Spinner size="sm" />
            </div>
          ) : (
            <p className="mt-2 text-3xl font-bold text-gray-900 dark:text-white">{value}</p>
          )}
        </div>
        <div className={`p-3 rounded-lg ${colorClasses[color]}`}>
          {icon}
        </div>
      </div>
    </div>
  )
}

function HealthStatusCard({ status, lastChecked, loading }: { 
  status: DashboardStats['healthStatus']
  lastChecked: Date | null
  loading?: boolean 
}) {
  const statusConfig = {
    healthy: {
      icon: <CheckCircle className="w-6 h-6" />,
      text: 'Healthy',
      color: 'text-green-600 dark:text-green-400',
      bgColor: 'bg-green-100 dark:bg-green-900/30',
    },
    unhealthy: {
      icon: <XCircle className="w-6 h-6" />,
      text: 'Unhealthy',
      color: 'text-red-600 dark:text-red-400',
      bgColor: 'bg-red-100 dark:bg-red-900/30',
    },
    unknown: {
      icon: <Activity className="w-6 h-6" />,
      text: 'Unknown',
      color: 'text-gray-600 dark:text-gray-400',
      bgColor: 'bg-gray-100 dark:bg-gray-800',
    },
  }

  const config = statusConfig[status]

  return (
    <div className="bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm font-medium text-gray-500 dark:text-gray-400">System Status</p>
          {loading ? (
            <div className="mt-2 h-9 flex items-center">
              <Spinner size="sm" />
            </div>
          ) : (
            <p className={`mt-2 text-2xl font-bold ${config.color}`}>{config.text}</p>
          )}
          {lastChecked && (
            <p className="mt-1 text-xs text-gray-400 dark:text-gray-500 flex items-center gap-1">
              <Clock className="w-3 h-3" />
              {lastChecked.toLocaleTimeString()}
            </p>
          )}
        </div>
        <div className={`p-3 rounded-lg ${config.bgColor} ${config.color}`}>
          {config.icon}
        </div>
      </div>
    </div>
  )
}

export default function DashboardPage() {
  const { user } = useAuthStore()
  const isAdmin = user?.role === 'admin'
  
  const [stats, setStats] = useState<DashboardStats>({
    collections: 0,
    documents: 0,
    users: 0,
    triggers: 0,
    healthStatus: 'unknown',
    lastChecked: null,
  })
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)

  const fetchStats = useCallback(async (isRefresh = false) => {
    if (isRefresh) setRefreshing(true)
    else setLoading(true)

    try {
      // Fetch collections
      let collections: { name: string }[] = []
      try {
        collections = await documentsApi.listCollections()
      } catch {
        collections = []
      }

      // Count documents per collection (sample first few)
      let totalDocs = 0
      for (const col of collections.slice(0, 5)) {
        try {
          const result = await documentsApi.query({ collection: col.name, limit: 100 })
          totalDocs += result.documents.length
          if (result.hasMore) totalDocs += 1 // Indicate more exist
        } catch {
          // Skip failed collections
        }
      }

      // Fetch users count (admin only)
      let usersCount = 0
      if (isAdmin) {
        try {
          const users = await adminApi.listUsers(100, 0)
          usersCount = users.length
        } catch {
          usersCount = 0
        }
      }

      // Check health
      let healthStatus: DashboardStats['healthStatus'] = 'unknown'
      try {
        const response = await api.get('/health')
        healthStatus = response.status === 200 ? 'healthy' : 'unhealthy'
      } catch {
        healthStatus = 'unhealthy'
      }

      setStats({
        collections: collections.length,
        documents: totalDocs,
        users: usersCount,
        triggers: 0, // TODO: Add triggers API
        healthStatus,
        lastChecked: new Date(),
      })
    } catch (err) {
      console.error('Failed to fetch stats:', err)
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [isAdmin])

  useEffect(() => {
    fetchStats()
  }, [fetchStats])

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">Dashboard</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            Welcome back, {user?.username || 'User'}
          </p>
        </div>
        <Button
          variant="secondary"
          onClick={() => fetchStats(true)}
          disabled={refreshing}
        >
          <RefreshCw className={`w-4 h-4 mr-2 ${refreshing ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
        <StatCard
          title="Collections"
          value={stats.collections}
          icon={<Database className="w-6 h-6" />}
          loading={loading}
          color="blue"
        />
        <StatCard
          title="Documents"
          value={stats.documents > 100 ? '100+' : stats.documents}
          icon={<FileText className="w-6 h-6" />}
          loading={loading}
          color="green"
        />
        {isAdmin && (
          <StatCard
            title="Users"
            value={stats.users}
            icon={<Users className="w-6 h-6" />}
            loading={loading}
            color="purple"
          />
        )}
        <StatCard
          title="Triggers"
          value={stats.triggers || '-'}
          icon={<Zap className="w-6 h-6" />}
          loading={loading}
          color="amber"
        />
      </div>

      {/* Health Status */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <HealthStatusCard 
          status={stats.healthStatus} 
          lastChecked={stats.lastChecked}
          loading={loading}
        />
        
        {/* Quick Links */}
        <div className="lg:col-span-2 bg-white dark:bg-gray-800 rounded-xl shadow-sm border border-gray-200 dark:border-gray-700 p-6">
          <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">Quick Actions</h3>
          <div className="grid grid-cols-2 gap-4">
            <a 
              href="/collections" 
              className="flex items-center gap-3 p-4 rounded-lg bg-gray-50 dark:bg-gray-700/50 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
            >
              <Database className="w-5 h-5 text-blue-500" />
              <span className="font-medium text-gray-700 dark:text-gray-300">Browse Data</span>
            </a>
            <a 
              href="/triggers" 
              className="flex items-center gap-3 p-4 rounded-lg bg-gray-50 dark:bg-gray-700/50 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
            >
              <Zap className="w-5 h-5 text-amber-500" />
              <span className="font-medium text-gray-700 dark:text-gray-300">Manage Triggers</span>
            </a>
            {isAdmin && (
              <>
                <a 
                  href="/users" 
                  className="flex items-center gap-3 p-4 rounded-lg bg-gray-50 dark:bg-gray-700/50 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
                >
                  <Users className="w-5 h-5 text-purple-500" />
                  <span className="font-medium text-gray-700 dark:text-gray-300">Manage Users</span>
                </a>
                <a 
                  href="/security" 
                  className="flex items-center gap-3 p-4 rounded-lg bg-gray-50 dark:bg-gray-700/50 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
                >
                  <Activity className="w-5 h-5 text-green-500" />
                  <span className="font-medium text-gray-700 dark:text-gray-300">Security Rules</span>
                </a>
              </>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
