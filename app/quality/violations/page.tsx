'use client'

import { useEffect, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DatabaseSelector } from '@/components/database-selector'
import {
  AlertCircle,
  AlertTriangle,
  Info,
  XCircle,
  CheckCircle,
  ChevronLeft,
  ChevronRight,
  Filter,
  Search
} from 'lucide-react'

interface Violation {
  id: number
  normalized_item_id: number
  rule_name: string
  category: string
  severity: string
  message: string
  recommendation: string
  field_name: string
  current_value: string
  resolved: boolean
  resolved_at: string | null
  resolved_by: string | null
  created_at: string
}

interface ViolationsResponse {
  violations: Violation[]
  total: number
  limit: number
  offset: number
}

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || 'http://localhost:8080'

// Severity configuration
const severityConfig = {
  critical: {
    label: 'Критический',
    icon: XCircle,
    color: 'bg-red-500 text-white',
    borderColor: 'border-red-500',
    textColor: 'text-red-600'
  },
  error: {
    label: 'Ошибка',
    icon: AlertCircle,
    color: 'bg-orange-500 text-white',
    borderColor: 'border-orange-500',
    textColor: 'text-orange-600'
  },
  warning: {
    label: 'Предупреждение',
    icon: AlertTriangle,
    color: 'bg-yellow-500 text-white',
    borderColor: 'border-yellow-500',
    textColor: 'text-yellow-600'
  },
  info: {
    label: 'Информация',
    icon: Info,
    color: 'bg-blue-500 text-white',
    borderColor: 'border-blue-500',
    textColor: 'text-blue-600'
  }
}

// Category configuration
const categoryConfig = {
  completeness: 'Полнота данных',
  accuracy: 'Точность',
  consistency: 'Согласованность',
  format: 'Формат'
}

export default function ViolationsPage() {
  const searchParams = useSearchParams()
  const [selectedDatabase, setSelectedDatabase] = useState<string>('')
  const [violations, setViolations] = useState<Violation[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Filters
  const [severityFilter, setSeverityFilter] = useState<string>('all')
  const [categoryFilter, setCategoryFilter] = useState<string>('all')
  const [showResolved, setShowResolved] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')

  // Pagination
  const [currentPage, setCurrentPage] = useState(1)
  const [itemsPerPage] = useState(20)

  // Resolving state
  const [resolvingId, setResolvingId] = useState<number | null>(null)

  useEffect(() => {
    const dbParam = searchParams.get('database')
    if (dbParam) {
      setSelectedDatabase(dbParam)
    }
  }, [searchParams])

  useEffect(() => {
    if (selectedDatabase) {
      fetchViolations()
    }
  }, [selectedDatabase, severityFilter, categoryFilter, showResolved, currentPage])

  const fetchViolations = async () => {
    setLoading(true)
    setError(null)

    try {
      const params = new URLSearchParams({
        database: selectedDatabase,
        limit: itemsPerPage.toString(),
        offset: ((currentPage - 1) * itemsPerPage).toString()
      })

      if (severityFilter !== 'all') {
        params.append('severity', severityFilter)
      }

      if (categoryFilter !== 'all') {
        params.append('category', categoryFilter)
      }

      const response = await fetch(
        `${API_BASE}/api/quality/violations?${params.toString()}`
      )

      if (!response.ok) {
        throw new Error('Failed to fetch violations')
      }

      const data: ViolationsResponse = await response.json()

      // Filter out resolved if needed
      let filteredViolations = data.violations || []
      if (!showResolved) {
        filteredViolations = filteredViolations.filter(v => !v.resolved)
      }

      setViolations(filteredViolations)
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  const handleResolveViolation = async (violationId: number) => {
    setResolvingId(violationId)

    try {
      const response = await fetch(
        `${API_BASE}/api/quality/violations/${violationId}`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify({
            resolved_by: 'User' // TODO: Get from auth context
          })
        }
      )

      if (!response.ok) {
        throw new Error('Failed to resolve violation')
      }

      // Refresh violations list
      await fetchViolations()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to resolve violation')
    } finally {
      setResolvingId(null)
    }
  }

  const getSeverityBadge = (severity: string) => {
    const config = severityConfig[severity as keyof typeof severityConfig]
    if (!config) return null

    const Icon = config.icon

    return (
      <Badge className={config.color}>
        <Icon className="w-3 h-3 mr-1" />
        {config.label}
      </Badge>
    )
  }

  const getCategoryBadge = (category: string) => {
    const label = categoryConfig[category as keyof typeof categoryConfig] || category
    return (
      <Badge variant="outline">
        {label}
      </Badge>
    )
  }

  const totalPages = Math.ceil(total / itemsPerPage)

  if (!selectedDatabase) {
    return (
      <div className="container mx-auto p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold">Нарушения качества</h1>
            <p className="text-muted-foreground mt-1">
              Просмотр и управление нарушениями правил качества данных
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Выберите базу данных</CardTitle>
            <CardDescription>
              Для просмотра нарушений выберите базу данных
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DatabaseSelector
              value={selectedDatabase}
              onValueChange={setSelectedDatabase}
            />
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="container mx-auto p-6 space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold">Нарушения качества</h1>
          <p className="text-muted-foreground mt-1">
            База данных: <span className="font-medium">{selectedDatabase}</span>
          </p>
        </div>
        <DatabaseSelector
          value={selectedDatabase}
          onValueChange={setSelectedDatabase}
        />
      </div>

      {/* Filters */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Filter className="w-5 h-5" />
            Фильтры
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div className="space-y-2">
              <Label>Серьезность</Label>
              <Select value={severityFilter} onValueChange={setSeverityFilter}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Все</SelectItem>
                  <SelectItem value="critical">Критический</SelectItem>
                  <SelectItem value="error">Ошибка</SelectItem>
                  <SelectItem value="warning">Предупреждение</SelectItem>
                  <SelectItem value="info">Информация</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Категория</Label>
              <Select value={categoryFilter} onValueChange={setCategoryFilter}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Все</SelectItem>
                  <SelectItem value="completeness">Полнота данных</SelectItem>
                  <SelectItem value="accuracy">Точность</SelectItem>
                  <SelectItem value="consistency">Согласованность</SelectItem>
                  <SelectItem value="format">Формат</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Статус</Label>
              <Select
                value={showResolved ? 'all' : 'unresolved'}
                onValueChange={(val) => setShowResolved(val === 'all')}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="unresolved">Не решено</SelectItem>
                  <SelectItem value="all">Все</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Поиск</Label>
              <div className="relative">
                <Search className="absolute left-2 top-2.5 h-4 w-4 text-muted-foreground" />
                <Input
                  placeholder="Поиск по правилу..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-8"
                />
              </div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Error Alert */}
      {error && (
        <Alert variant="destructive">
          <AlertCircle className="h-4 w-4" />
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {/* Violations List */}
      {loading ? (
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="flex flex-col items-center gap-2">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
              <p className="text-sm text-muted-foreground">Загрузка нарушений...</p>
            </div>
          </CardContent>
        </Card>
      ) : violations.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <CheckCircle className="w-12 h-12 text-green-500 mb-4" />
            <h3 className="text-lg font-semibold mb-2">Нарушений не найдено</h3>
            <p className="text-sm text-muted-foreground">
              {showResolved
                ? 'В базе данных нет нарушений качества'
                : 'Все нарушения были решены'}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-4">
          {/* Summary */}
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center justify-between">
                <p className="text-sm text-muted-foreground">
                  Найдено нарушений: <span className="font-bold text-foreground">{total}</span>
                </p>
                <p className="text-sm text-muted-foreground">
                  Страница {currentPage} из {totalPages}
                </p>
              </div>
            </CardContent>
          </Card>

          {/* Violations Cards */}
          {violations
            .filter(v =>
              searchQuery === '' ||
              v.rule_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
              v.message.toLowerCase().includes(searchQuery.toLowerCase())
            )
            .map((violation) => {
              const severityConf = severityConfig[violation.severity as keyof typeof severityConfig]

              return (
                <Card
                  key={violation.id}
                  className={`border-l-4 ${severityConf?.borderColor || 'border-gray-500'} ${
                    violation.resolved ? 'opacity-60' : ''
                  }`}
                >
                  <CardHeader>
                    <div className="flex items-start justify-between">
                      <div className="space-y-2 flex-1">
                        <div className="flex items-center gap-2">
                          {getSeverityBadge(violation.severity)}
                          {getCategoryBadge(violation.category)}
                          {violation.resolved && (
                            <Badge variant="outline" className="bg-green-50 text-green-700 border-green-200">
                              <CheckCircle className="w-3 h-3 mr-1" />
                              Решено
                            </Badge>
                          )}
                        </div>
                        <CardTitle className="text-lg">
                          {violation.rule_name}
                        </CardTitle>
                        <CardDescription>
                          ID записи: #{violation.normalized_item_id}
                          {violation.field_name && ` • Поле: ${violation.field_name}`}
                        </CardDescription>
                      </div>
                      {!violation.resolved && (
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => handleResolveViolation(violation.id)}
                          disabled={resolvingId === violation.id}
                        >
                          {resolvingId === violation.id ? 'Решение...' : 'Решить'}
                        </Button>
                      )}
                    </div>
                  </CardHeader>
                  <CardContent className="space-y-4">
                    <div>
                      <h4 className="text-sm font-medium mb-1">Сообщение:</h4>
                      <p className="text-sm text-muted-foreground">{violation.message}</p>
                    </div>

                    {violation.current_value && (
                      <div>
                        <h4 className="text-sm font-medium mb-1">Текущее значение:</h4>
                        <code className="text-sm bg-muted px-2 py-1 rounded">
                          {violation.current_value}
                        </code>
                      </div>
                    )}

                    {violation.recommendation && (
                      <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
                        <h4 className="text-sm font-medium text-blue-900 mb-1">
                          Рекомендация:
                        </h4>
                        <p className="text-sm text-blue-700">{violation.recommendation}</p>
                      </div>
                    )}

                    {violation.resolved && (
                      <div className="text-xs text-muted-foreground pt-2 border-t">
                        Решено {violation.resolved_by}
                        {violation.resolved_at && ` • ${new Date(violation.resolved_at).toLocaleString('ru-RU')}`}
                      </div>
                    )}

                    <div className="text-xs text-muted-foreground">
                      Создано: {new Date(violation.created_at).toLocaleString('ru-RU')}
                    </div>
                  </CardContent>
                </Card>
              )
            })}

          {/* Pagination */}
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center justify-between">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCurrentPage(prev => Math.max(1, prev - 1))}
                  disabled={currentPage === 1}
                >
                  <ChevronLeft className="w-4 h-4 mr-1" />
                  Назад
                </Button>

                <div className="flex items-center gap-2">
                  {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
                    let pageNum
                    if (totalPages <= 5) {
                      pageNum = i + 1
                    } else if (currentPage <= 3) {
                      pageNum = i + 1
                    } else if (currentPage >= totalPages - 2) {
                      pageNum = totalPages - 4 + i
                    } else {
                      pageNum = currentPage - 2 + i
                    }

                    return (
                      <Button
                        key={pageNum}
                        variant={currentPage === pageNum ? 'default' : 'outline'}
                        size="sm"
                        onClick={() => setCurrentPage(pageNum)}
                      >
                        {pageNum}
                      </Button>
                    )
                  })}
                </div>

                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setCurrentPage(prev => Math.min(totalPages, prev + 1))}
                  disabled={currentPage === totalPages}
                >
                  Вперед
                  <ChevronRight className="w-4 h-4 ml-1" />
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}
    </div>
  )
}
