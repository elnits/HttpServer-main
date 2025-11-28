'use client'

import { useEffect, useState } from 'react'
import { useSearchParams } from 'next/navigation'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { DatabaseSelector } from '@/components/database-selector'
import {
  AlertCircle,
  CheckCircle,
  ChevronLeft,
  ChevronRight,
  Copy,
  GitMerge,
  Star,
  TrendingUp
} from 'lucide-react'

interface DuplicateItem {
  id: number
  normalized_name: string
  code: string
  category: string
  kpved_code: string
  quality_score: number
  processing_level: string
  merged_count: number
}

interface DuplicateGroup {
  id: number
  detection_method?: string  // Для обратной совместимости
  duplicate_type?: string   // Основное поле из API
  similarity_score: number
  suggested_master_id: number
  item_count?: number        // Может быть вычислено из items.length
  item_ids?: number[]        // Массив ID элементов
  items?: DuplicateItem[]    // Полные данные элементов
  merged: boolean
  merged_at: string | null
  created_at: string
  reason?: string            // Причина определения как дубликат
}

interface DuplicatesResponse {
  groups: DuplicateGroup[]
  total: number
  limit: number
  offset: number
}

const API_BASE = process.env.NEXT_PUBLIC_API_BASE || 'http://localhost:8080'

// Detection method configuration
const methodConfig = {
  exact: {
    label: 'Точное совпадение',
    color: 'bg-red-500 text-white',
    icon: '🔴'
  },
  exact_code: {
    label: 'Точное совпадение кода',
    color: 'bg-red-500 text-white',
    icon: '🔴'
  },
  exact_name: {
    label: 'Точное совпадение имени',
    color: 'bg-orange-500 text-white',
    icon: '🟠'
  },
  semantic: {
    label: 'Семантическое сходство',
    color: 'bg-blue-500 text-white',
    icon: '🔵'
  },
  phonetic: {
    label: 'Фонетическое сходство',
    color: 'bg-purple-500 text-white',
    icon: '🟣'
  },
  word_based: {
    label: 'Группировка по словам',
    color: 'bg-green-500 text-white',
    icon: '🟢'
  },
  mixed: {
    label: 'Смешанный тип',
    color: 'bg-gray-500 text-white',
    icon: '⚫'
  }
}

export default function DuplicatesPage() {
  const searchParams = useSearchParams()
  const [selectedDatabase, setSelectedDatabase] = useState<string>('')
  const [groups, setGroups] = useState<DuplicateGroup[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Filters
  const [showMerged, setShowMerged] = useState(false)

  // Pagination
  const [currentPage, setCurrentPage] = useState(1)
  const [itemsPerPage] = useState(10)

  // Merging state
  const [mergingId, setMergingId] = useState<number | null>(null)

  useEffect(() => {
    const dbParam = searchParams.get('database')
    if (dbParam) {
      setSelectedDatabase(dbParam)
    }
  }, [searchParams])

  useEffect(() => {
    if (selectedDatabase) {
      fetchDuplicates()
    }
  }, [selectedDatabase, showMerged, currentPage])

  const fetchDuplicates = async () => {
    setLoading(true)
    setError(null)

    try {
      const params = new URLSearchParams({
        database: selectedDatabase,
        limit: itemsPerPage.toString(),
        offset: ((currentPage - 1) * itemsPerPage).toString()
      })

      if (!showMerged) {
        params.append('unmerged', 'true')
      }

      const response = await fetch(
        `${API_BASE}/api/quality/duplicates?${params.toString()}`
      )

      if (!response.ok) {
        throw new Error('Failed to fetch duplicates')
      }

      const data: DuplicatesResponse = await response.json()
      setGroups(data.groups || [])
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error')
    } finally {
      setLoading(false)
    }
  }

  const handleMergeGroup = async (groupId: number) => {
    setMergingId(groupId)

    try {
      const response = await fetch(
        `${API_BASE}/api/quality/duplicates/${groupId}/merge`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          }
        }
      )

      if (!response.ok) {
        throw new Error('Failed to merge duplicate group')
      }

      // Refresh duplicates list
      await fetchDuplicates()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to merge group')
    } finally {
      setMergingId(null)
    }
  }

  const getMethodBadge = (group: DuplicateGroup) => {
    // Используем duplicate_type если есть, иначе detection_method
    const method = group.duplicate_type || group.detection_method || 'unknown'
    const config = methodConfig[method as keyof typeof methodConfig]
    
    if (!config) {
      // Fallback для неизвестных типов
      return (
        <Badge className="bg-gray-500 text-white">
          <span className="mr-1">❓</span>
          {method}
        </Badge>
      )
    }

    return (
      <Badge className={config.color}>
        <span className="mr-1">{config.icon}</span>
        {config.label}
      </Badge>
    )
  }

  const getSimilarityBadge = (score: number) => {
    const percentage = Math.round(score * 100)
    let color = 'bg-gray-500'

    if (percentage >= 95) color = 'bg-red-500'
    else if (percentage >= 90) color = 'bg-orange-500'
    else if (percentage >= 85) color = 'bg-yellow-500'
    else color = 'bg-blue-500'

    return (
      <Badge className={`${color} text-white`}>
        {percentage}% схожесть
      </Badge>
    )
  }

  const getQualityBadge = (score: number) => {
    const percentage = Math.round(score * 100)
    let variant: 'default' | 'destructive' | 'outline' = 'outline'

    if (percentage >= 90) variant = 'default'
    else if (percentage < 70) variant = 'destructive'

    return (
      <Badge variant={variant}>
        {percentage}% качество
      </Badge>
    )
  }

  const getProcessingLevelBadge = (level: string) => {
    const labels: Record<string, string> = {
      basic: 'База',
      ai_enhanced: 'AI',
      benchmark: 'Эталон'
    }

    const colors: Record<string, string> = {
      basic: 'bg-gray-500',
      ai_enhanced: 'bg-blue-500',
      benchmark: 'bg-green-500'
    }

    return (
      <Badge className={`${colors[level] || 'bg-gray-500'} text-white text-xs`}>
        {labels[level] || level}
      </Badge>
    )
  }

  const totalPages = Math.ceil(total / itemsPerPage)

  if (!selectedDatabase) {
    return (
      <div className="container mx-auto p-6 space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold">Дубликаты</h1>
            <p className="text-muted-foreground mt-1">
              Управление группами дубликатов и объединение записей
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Выберите базу данных</CardTitle>
            <CardDescription>
              Для просмотра дубликатов выберите базу данных
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
            <Copy className="w-8 h-8" />
            Дубликаты
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

      {/* Controls */}
      <Card>
        <CardContent className="pt-6">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <Button
                variant={showMerged ? 'outline' : 'default'}
                size="sm"
                onClick={() => setShowMerged(false)}
              >
                Требуют объединения
              </Button>
              <Button
                variant={showMerged ? 'default' : 'outline'}
                size="sm"
                onClick={() => setShowMerged(true)}
              >
                Все группы
              </Button>
            </div>

            <div className="text-sm text-muted-foreground">
              Найдено групп: <span className="font-bold text-foreground">{total}</span>
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

      {/* Duplicates List */}
      {loading ? (
        <Card>
          <CardContent className="flex items-center justify-center py-12">
            <div className="flex flex-col items-center gap-2">
              <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
              <p className="text-sm text-muted-foreground">Загрузка дубликатов...</p>
            </div>
          </CardContent>
        </Card>
      ) : groups.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12">
            <CheckCircle className="w-12 h-12 text-green-500 mb-4" />
            <h3 className="text-lg font-semibold mb-2">Дубликатов не найдено</h3>
            <p className="text-sm text-muted-foreground">
              {showMerged
                ? 'В базе данных нет дубликатов'
                : 'Все дубликаты были объединены'}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-6">
          {/* Duplicate Groups */}
          {groups.map((group) => {
            const masterItem = group.items?.find(item => item.id === group.suggested_master_id)

            return (
              <Card
                key={group.id}
                className={`border-l-4 ${
                  group.merged ? 'border-green-500 opacity-60' : 'border-orange-500'
                }`}
              >
                <CardHeader>
                  <div className="flex items-start justify-between">
                    <div className="space-y-2 flex-1">
                      <div className="flex items-center gap-2 flex-wrap">
                        {getMethodBadge(group)}
                        {getSimilarityBadge(group.similarity_score)}
                        <Badge variant="outline">
                          {group.item_count || group.items?.length || group.item_ids?.length || 0} записей
                        </Badge>
                        {group.reason && (
                          <Badge variant="outline" className="text-xs">
                            {group.reason}
                          </Badge>
                        )}
                        {group.merged && (
                          <Badge className="bg-green-500 text-white">
                            <CheckCircle className="w-3 h-3 mr-1" />
                            Объединено
                          </Badge>
                        )}
                      </div>
                      <CardTitle className="text-lg">
                        Группа дубликатов #{group.id}
                      </CardTitle>
                      <CardDescription>
                        Создано: {new Date(group.created_at).toLocaleString('ru-RU')}
                        {group.merged_at && ` • Объединено: ${new Date(group.merged_at).toLocaleString('ru-RU')}`}
                      </CardDescription>
                    </div>
                    {!group.merged && (
                      <Button
                        size="sm"
                        onClick={() => handleMergeGroup(group.id)}
                        disabled={mergingId === group.id}
                        className="bg-green-600 hover:bg-green-700"
                      >
                        <GitMerge className="w-4 h-4 mr-2" />
                        {mergingId === group.id ? 'Объединение...' : 'Объединить'}
                      </Button>
                    )}
                  </div>
                </CardHeader>
                <CardContent className="space-y-4">
                  {/* Master Record */}
                  {masterItem && (
                    <div className="bg-yellow-50 border-2 border-yellow-300 rounded-lg p-4">
                      <div className="flex items-center gap-2 mb-3">
                        <Star className="w-5 h-5 text-yellow-600 fill-yellow-600" />
                        <h4 className="font-semibold text-yellow-900">Рекомендуемая мастер-запись</h4>
                      </div>
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm">
                        <div>
                          <span className="text-muted-foreground">ID:</span>{' '}
                          <span className="font-mono">#{masterItem.id}</span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Код:</span>{' '}
                          <code className="bg-yellow-100 px-2 py-0.5 rounded">
                            {masterItem.code || 'N/A'}
                          </code>
                        </div>
                        <div className="md:col-span-2">
                          <span className="text-muted-foreground">Название:</span>{' '}
                          <span className="font-medium">{masterItem.normalized_name}</span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Категория:</span>{' '}
                          <span>{masterItem.category}</span>
                        </div>
                        <div>
                          <span className="text-muted-foreground">КПВЭД:</span>{' '}
                          <code className="bg-yellow-100 px-2 py-0.5 rounded">
                            {masterItem.kpved_code || 'N/A'}
                          </code>
                        </div>
                        <div className="flex items-center gap-2">
                          {getQualityBadge(masterItem.quality_score)}
                          {getProcessingLevelBadge(masterItem.processing_level)}
                        </div>
                        {masterItem.merged_count > 0 && (
                          <div>
                            <Badge variant="outline" className="bg-blue-50">
                              <TrendingUp className="w-3 h-3 mr-1" />
                              {masterItem.merged_count} объединений
                            </Badge>
                          </div>
                        )}
                      </div>
                    </div>
                  )}

                  {/* Duplicate Items */}
                  <div>
                    <h4 className="font-semibold mb-3 text-sm text-muted-foreground">
                      Все записи в группе:
                    </h4>
                    <div className="space-y-2">
                      {group.items?.map((item) => (
                        <div
                          key={item.id}
                          className={`border rounded-lg p-3 ${
                            item.id === group.suggested_master_id
                              ? 'bg-yellow-50 border-yellow-200'
                              : 'bg-white'
                          }`}
                        >
                          <div className="grid grid-cols-1 md:grid-cols-3 gap-2 text-sm">
                            <div className="md:col-span-3 flex items-start justify-between">
                              <div className="flex-1">
                                <div className="flex items-center gap-2 mb-1">
                                  <span className="font-mono text-xs text-muted-foreground">
                                    #{item.id}
                                  </span>
                                  {item.id === group.suggested_master_id && (
                                    <Star className="w-4 h-4 text-yellow-600 fill-yellow-600" />
                                  )}
                                </div>
                                <div className="font-medium">{item.normalized_name}</div>
                              </div>
                            </div>
                            <div>
                              <span className="text-muted-foreground text-xs">Код:</span>{' '}
                              <code className="text-xs bg-muted px-1.5 py-0.5 rounded">
                                {item.code || 'N/A'}
                              </code>
                            </div>
                            <div>
                              <span className="text-muted-foreground text-xs">КПВЭД:</span>{' '}
                              <code className="text-xs bg-muted px-1.5 py-0.5 rounded">
                                {item.kpved_code || 'N/A'}
                              </code>
                            </div>
                            <div className="flex items-center gap-2">
                              {getQualityBadge(item.quality_score)}
                              {getProcessingLevelBadge(item.processing_level)}
                            </div>
                          </div>
                        </div>
                      ))}
                    </div>
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
