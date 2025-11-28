'use client'

import { useEffect, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Label } from '@/components/ui/label'
import { DatabaseSelector } from '@/components/database-selector'
import {
  AlertCircle,
  CheckCircle,
  ChevronLeft,
  ChevronRight,
  Lightbulb,
  Zap,
  ArrowRight,
  TrendingUp,
  Settings,
  RefreshCw,
  GitMerge,
  Eye
} from 'lucide-react'
import { Progress } from '@/components/ui/progress'

interface Suggestion {
  id: number
  normalized_item_id: number
  type: string
  priority: string
  field: string
  current_value: string
  suggested_value: string
  confidence: number
  reasoning: string
  auto_applyable: boolean
  applied: boolean
  applied_at: string | null
  created_at: string
}

interface SuggestionsResponse {
  suggestions: Suggestion[]
  total: number
  limit: number
  offset: number
}

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || 'http://localhost:8080'

// Suggestion type configuration
const typeConfig = {
  set_value: {
    label: 'Установить значение',
    icon: Settings,
    color: 'bg-blue-500 text-white',
    description: 'Установить новое значение поля'
  },
  correct_format: {
    label: 'Исправить формат',
    icon: RefreshCw,
    color: 'bg-purple-500 text-white',
    description: 'Исправить формат данных'
  },
  reprocess: {
    label: 'Повторная обработка',
    icon: RefreshCw,
    color: 'bg-orange-500 text-white',
    description: 'Повторно обработать запись'
  },
  merge: {
    label: 'Объединить',
    icon: GitMerge,
    color: 'bg-green-500 text-white',
    description: 'Объединить с другой записью'
  },
  review: {
    label: 'Требует проверки',
    icon: Eye,
    color: 'bg-yellow-500 text-white',
    description: 'Требуется ручная проверка'
  }
}

// Priority configuration
const priorityConfig = {
  critical: {
    label: 'Критический',
    color: 'bg-red-500 text-white',
    order: 4
  },
  high: {
    label: 'Высокий',
    color: 'bg-orange-500 text-white',
    order: 3
  },
  medium: {
    label: 'Средний',
    color: 'bg-yellow-500 text-white',
    order: 2
  },
  low: {
    label: 'Низкий',
    color: 'bg-blue-500 text-white',
    order: 1
  }
}

export default function SuggestionsPage() {
  const searchParams = useSearchParams()
  const [selectedDatabase, setSelectedDatabase] = useState<string>('')
  const [suggestions, setSuggestions] = useState<Suggestion[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Filters
  const [priorityFilter, setPriorityFilter] = useState<string>('all')
  const [typeFilter, setTypeFilter] = useState<string>('all')
  const [showApplied, setShowApplied] = useState(false)
  const [autoApplyableOnly, setAutoApplyableOnly] = useState(false)

  // Pagination
  const [currentPage, setCurrentPage] = useState(1)
  const [itemsPerPage] = useState(20)

  // Applying state
  const [applyingId, setApplyingId] = useState<number | null>(null)

  useEffect(() => {
    const dbParam = searchParams.get('database')
    if (dbParam) {
      setSelectedDatabase(dbParam)
    }
  }, [searchParams])

  useEffect(() => {
    if (selectedDatabase) {
      fetchSuggestions()
    }
  }, [selectedDatabase, priorityFilter, typeFilter, showApplied, autoApplyableOnly, currentPage])

  const fetchSuggestions = async () => {
    setLoading(true)
    setError(null)

    try {
      const params = new URLSearchParams({
        database: selectedDatabase,
        limit: itemsPerPage.toString(),
        offset: ((currentPage - 1) * itemsPerPage).toString()
      })

      if (priorityFilter !== 'all') {
        params.append('priority', priorityFilter)
      }

      if (!showApplied) {
        params.append('applied', 'false')
      }

      if (autoApplyableOnly) {
        params.append('auto_applyable', 'true')
      }

      const response = await fetch(
        `${API_BASE}/api/quality/suggestions?${params.toString()}`
      )

      if (!response.ok) {
        throw new Error('Failed to fetch suggestions')
      }

      const data: SuggestionsResponse = await response.json()

      // Filter by type if needed
      let filteredSuggestions = data.suggestions || []
      if (typeFilter !== 'all') {
        filteredSuggestions = filteredSuggestions.filter(s => s.type === typeFilter)
      }

      setSuggestions(filteredSuggestions)
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  const handleApplySuggestion = async (suggestionId: number) => {
    setApplyingId(suggestionId)

    try {
      const response = await fetch(
        `${API_BASE}/api/quality/suggestions/${suggestionId}/apply`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          }
        }
      )

      if (!response.ok) {
        throw new Error('Failed to apply suggestion')
      }

      // Refresh suggestions list
      await fetchSuggestions()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to apply suggestion')
    } finally {
      setApplyingId(null)
    }
  }

  const getTypeBadge = (type: string) => {
    const config = typeConfig[type as keyof typeof typeConfig]
    if (!config) return null

    const Icon = config.icon

    return (
      <Badge className={config.color}>
        <Icon className="w-3 h-3 mr-1" />
        {config.label}
      </Badge>
    )
  }

  const getPriorityBadge = (priority: string) => {
    const config = priorityConfig[priority as keyof typeof priorityConfig]
    if (!config) return null

    return (
      <Badge className={config.color}>
        {config.label}
      </Badge>
    )
  }

  const getConfidenceBadge = (confidence: number) => {
    const percentage = Math.round(confidence * 100)
    let color = 'bg-gray-500'

    if (percentage >= 90) color = 'bg-green-500'
    else if (percentage >= 80) color = 'bg-blue-500'
    else if (percentage >= 70) color = 'bg-yellow-500'
    else color = 'bg-orange-500'

    return (
      <Badge className={`${color} text-white`}>
        {percentage}% уверенность
      </Badge>
    )
  }

  const totalPages = Math.ceil(total / itemsPerPage)

  if (!selectedDatabase) {
    return (
      <div className="container mx-auto p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold">Предложения по улучшению</h1>
            <p className="text-muted-foreground mt-1">
              Автоматические рекомендации для повышения качества данных
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Выберите базу данных</CardTitle>
            <CardDescription>
              Для просмотра предложений выберите базу данных
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
          <h1 className="text-3xl font-bold flex items-center gap-2">
            <Lightbulb className="w-8 h-8 text-yellow-500" />
            Предложения по улучшению
          </h1>
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
          <CardTitle>Фильтры</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
            <div className="space-y-2">
              <Label>Приоритет</Label>
              <Select value={priorityFilter} onValueChange={setPriorityFilter}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Все</SelectItem>
                  <SelectItem value="critical">Критический</SelectItem>
                  <SelectItem value="high">Высокий</SelectItem>
                  <SelectItem value="medium">Средний</SelectItem>
                  <SelectItem value="low">Низкий</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Тип</Label>
              <Select value={typeFilter} onValueChange={setTypeFilter}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Все</SelectItem>
                  <SelectItem value="set_value">Установить значение</SelectItem>
                  <SelectItem value="correct_format">Исправить формат</SelectItem>
                  <SelectItem value="reprocess">Повторная обработка</SelectItem>
                  <SelectItem value="merge">Объединить</SelectItem>
                  <SelectItem value="review">Требует проверки</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Статус</Label>
              <Select
                value={showApplied ? 'all' : 'pending'}
                onValueChange={(val) => setShowApplied(val === 'all')}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="pending">Ожидают применения</SelectItem>
                  <SelectItem value="all">Все</SelectItem>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>Автоприменяемые</Label>
              <Select
                value={autoApplyableOnly ? 'yes' : 'all'}
                onValueChange={(val) => setAutoApplyableOnly(val === 'yes')}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Все</SelectItem>
                  <SelectItem value="yes">Только автоприменяемые</SelectItem>
                </SelectContent>
              </Select>
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

      {/* Suggestions List */}
      {loading ? (
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="flex flex-col items-center gap-2">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
              <p className="text-sm text-muted-foreground">Загрузка предложений...</p>
            </div>
          </CardContent>
        </Card>
      ) : suggestions.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <CheckCircle className="w-12 h-12 text-green-500 mb-4" />
            <h3 className="text-lg font-semibold mb-2">Предложений не найдено</h3>
            <p className="text-sm text-muted-foreground">
              {showApplied
                ? 'В базе данных нет предложений по улучшению'
                : 'Все предложения были применены'}
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
                  Найдено предложений: <span className="font-bold text-foreground">{total}</span>
                </p>
                <p className="text-sm text-muted-foreground">
                  Страница {currentPage} из {totalPages}
                </p>
              </div>
            </CardContent>
          </Card>

          {/* Suggestions Cards */}
          {suggestions.map((suggestion) => {
            const typeConf = typeConfig[suggestion.type as keyof typeof typeConfig]

            return (
              <Card
                key={suggestion.id}
                className={`border-l-4 ${
                  suggestion.applied
                    ? 'border-green-500 opacity-60'
                    : suggestion.auto_applyable
                    ? 'border-blue-500'
                    : 'border-yellow-500'
                }`}
              >
                <CardHeader>
                  <div className="flex items-start justify-between">
                    <div className="space-y-2 flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        {getPriorityBadge(suggestion.priority)}
                        {getTypeBadge(suggestion.type)}
                        {getConfidenceBadge(suggestion.confidence)}
                        {suggestion.auto_applyable && !suggestion.applied && (
                          <Badge className="bg-blue-500 text-white">
                            <Zap className="w-3 h-3 mr-1" />
                            Автоприменяемо
                          </Badge>
                        )}
                        {suggestion.applied && (
                          <Badge className="bg-green-500 text-white">
                            <CheckCircle className="w-3 h-3 mr-1" />
                            Применено
                          </Badge>
                        )}
                      </div>
                      <CardTitle className="text-lg">
                        {typeConf?.description || suggestion.type}
                      </CardTitle>
                      <CardDescription>
                        ID записи: #{suggestion.normalized_item_id} • Поле: {suggestion.field}
                      </CardDescription>
                    </div>
                    {!suggestion.applied && (
                      <Button
                        size="sm"
                        onClick={() => handleApplySuggestion(suggestion.id)}
                        disabled={applyingId === suggestion.id}
                        className="bg-green-600 hover:bg-green-700"
                      >
                        {applyingId === suggestion.id ? 'Применение...' : 'Применить'}
                      </Button>
                    )}
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  {/* Current vs Suggested Value */}
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <h4 className="text-sm font-medium text-muted-foreground">
                        Текущее значение:
                      </h4>
                      <div className="bg-red-50 border border-red-200 rounded-lg p-3">
                        <code className="text-sm text-red-900 break-all">
                          {suggestion.current_value || '<пусто>'}
                        </code>
                      </div>
                    </div>

                    <div className="space-y-2">
                      <h4 className="text-sm font-medium text-muted-foreground flex items-center gap-2">
                        <ArrowRight className="w-4 h-4" />
                        Предлагаемое значение:
                      </h4>
                      <div className="bg-green-50 border border-green-200 rounded-lg p-3">
                        <code className="text-sm text-green-900 break-all">
                          {suggestion.suggested_value}
                        </code>
                      </div>
                    </div>
                  </div>

                  {/* Confidence Bar */}
                  <div className="space-y-2">
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">Уверенность:</span>
                      <span className="font-medium">
                        {Math.round(suggestion.confidence * 100)}%
                      </span>
                    </div>
                    <Progress value={suggestion.confidence * 100} className="h-2" />
                  </div>

                  {/* Reasoning */}
                  {suggestion.reasoning && (
                    <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
                      <h4 className="text-sm font-medium text-blue-900 mb-1 flex items-center gap-2">
                        <TrendingUp className="w-4 h-4" />
                        Обоснование:
                      </h4>
                      <p className="text-sm text-blue-700">{suggestion.reasoning}</p>
                    </div>
                  )}

                  {/* Metadata */}
                  <div className="text-xs text-muted-foreground pt-2 border-t flex items-center justify-between">
                    <span>Создано: {new Date(suggestion.created_at).toLocaleString('ru-RU')}</span>
                    {suggestion.applied_at && (
                      <span>Применено: {new Date(suggestion.applied_at).toLocaleString('ru-RU')}</span>
                    )}
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
