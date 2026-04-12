import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:webapp/theme.dart';
import 'package:webapp/services/api_service.dart';
import 'package:webapp/services/file_download.dart';
import 'package:webapp/models/models.dart';
import 'package:webapp/widgets/process_result_drawer.dart';
import 'package:webapp/widgets/queue_stage_overview.dart';
import 'package:intl/intl.dart';

class DashboardScreen extends StatefulWidget {
  const DashboardScreen({super.key});

  @override
  State<DashboardScreen> createState() => _DashboardScreenState();
}

class _DashboardScreenState extends State<DashboardScreen> {
  bool _isLoading = true;
  String? _error;
  DashboardStats? _stats;
  Timer? _drawerCleanupTimer;
  QueueItem? _selectedResultItem;
  bool _isResultDrawerOpen = false;
  int? _applyingItemId;
  String _processingMode = 'manual';
  bool _hasCompletedTag = false;
  Set<String> _configuredStageKeys = <String>{'extract_text'};

  @override
  void initState() {
    super.initState();
    _loadData();
  }

  @override
  void dispose() {
    _drawerCleanupTimer?.cancel();
    super.dispose();
  }

  Future<void> _loadData() async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final api = context.read<ApiService>();
      final responses = await Future.wait([
        api.getDashboard(),
        api.getConfig(),
      ]);
      final stats = responses[0] as DashboardStats;
      final config = responses[1] as BackendConfig;
      if (mounted) {
        setState(() {
          _stats = stats;
          _processingMode = config.engine.processingMode;
          _hasCompletedTag = config.process.processCompletedTag
              .trim()
              .isNotEmpty;
          _configuredStageKeys = configuredQueueStageKeys(config.process);
          _selectedResultItem = _refreshSelectedItem(
            stats.recentRuns,
            _selectedResultItem,
          );
          _isLoading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Failed to load dashboard: \$e';
          _isLoading = false;
        });
      }
    }
  }

  Future<void> _applyItem(QueueItem item) async {
    if (_applyingItemId != null) {
      return;
    }

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          title: const Text('Apply suggestions?'),
          content: Text(
            'This writes the suggested metadata back to Paperless for "${item.documentTitle}" and also applies the configured completed tag.',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(false),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(dialogContext).pop(true),
              child: const Text('Apply'),
            ),
          ],
        );
      },
    );

    if (confirmed != true || !mounted) {
      return;
    }

    setState(() {
      _applyingItemId = item.id;
    });

    try {
      final api = context.read<ApiService>();
      final updatedItem = await api.applyQueueItem(item.id);

      if (!mounted) {
        return;
      }

      setState(() {
        final stats = _stats;
        if (stats != null) {
          final updatedRuns = List<QueueItem>.from(stats.recentRuns);
          final index = updatedRuns.indexWhere(
            (entry) => entry.id == updatedItem.id,
          );
          if (index >= 0) {
            updatedRuns[index] = updatedItem;
            _stats = DashboardStats(
              queuedCount: stats.queuedCount,
              averageProcessingMs: stats.averageProcessingMs,
              processingSuccessRate: stats.processingSuccessRate,
              recentRuns: updatedRuns,
            );
          }
        }
        if (_selectedResultItem?.id == updatedItem.id) {
          _selectedResultItem = updatedItem;
        }
        _applyingItemId = null;
      });
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            updatedItem.appliedSummary?.trim().isNotEmpty == true
                ? updatedItem.appliedSummary!
                : 'Suggestions applied successfully.',
          ),
        ),
      );
      await _loadData();
    } catch (e) {
      if (!mounted) {
        return;
      }

      setState(() {
        _applyingItemId = null;
      });
      final errorMessage = e is ApiException ? e.message : e.toString();
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('Apply failed: $errorMessage')));
      await _loadData();
    }
  }

  void _openResultDrawer(QueueItem item) {
    if (!item.canViewResultDetails) {
      return;
    }

    _drawerCleanupTimer?.cancel();
    setState(() {
      _selectedResultItem = item;
      _isResultDrawerOpen = true;
    });
  }

  void _closeResultDrawer() {
    if (_selectedResultItem == null && !_isResultDrawerOpen) {
      return;
    }

    _drawerCleanupTimer?.cancel();
    setState(() {
      _isResultDrawerOpen = false;
    });
    _drawerCleanupTimer = Timer(processResultDrawerTransitionDuration, () {
      if (!mounted) {
        return;
      }
      setState(() {
        _selectedResultItem = null;
      });
    });
  }

  QueueItem? _refreshSelectedItem(List<QueueItem> items, QueueItem? current) {
    if (current == null) {
      return null;
    }

    for (final item in items) {
      if (item.id == current.id) {
        return item;
      }
    }

    return current;
  }

  Future<void> _exportCsv() async {
    final stats = _stats;
    if (stats == null || stats.recentRuns.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('There is no processing history to export.'),
        ),
      );
      return;
    }

    final rows = <String>[
      'queue_id,document_id,document_title,status,source,used_llm,used_vision_llm,requested_at,completed_at,processing_duration_ms,attempts,result_summary',
      ...stats.recentRuns.map((item) {
        return [
          item.id.toString(),
          (item.documentId ?? '').toString(),
          _csvField(item.documentTitle),
          _csvField(item.status),
          _csvField(item.source),
          _csvField(item.usedLlm ?? ''),
          _csvField(item.usedVisionLlm ?? ''),
          _csvField(
            DateTime.fromMillisecondsSinceEpoch(
              item.requestedAtMs,
            ).toIso8601String(),
          ),
          _csvField(item.completedAt?.toIso8601String() ?? ''),
          (item.processingDurationMs ?? '').toString(),
          item.attempts.toString(),
          _csvField(item.resultSummary ?? ''),
        ].join(',');
      }),
    ];

    final fileName =
        'processing-history-${DateTime.now().millisecondsSinceEpoch}.csv';
    final downloaded = await downloadTextFile(
      fileName: fileName,
      content: rows.join('\n'),
      mimeType: 'text/csv;charset=utf-8',
    );

    if (!mounted) {
      return;
    }
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(
          downloaded
              ? 'Exported processing history as $fileName.'
              : 'CSV export is only available in the web build.',
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return const Center(child: CircularProgressIndicator());
    }

    if (_error != null) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Text(_error!, style: const TextStyle(color: TailwindColors.error)),
            const SizedBox(height: 16),
            ElevatedButton(onPressed: _loadData, child: const Text('Retry')),
          ],
        ),
      );
    }

    final stats = _stats!;

    return Stack(
      children: [
        SingleChildScrollView(
          padding: const EdgeInsets.all(32.0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: _buildStatCard(
                      'Documents in Queue',
                      Icons.layers,
                      stats.queuedCount.toString(),
                      'Active',
                      true,
                    ),
                  ),
                  const SizedBox(width: 24),
                  Expanded(
                    child: _buildStatCard(
                      'Avg. Processing Time',
                      Icons.timer,
                      '${(stats.averageProcessingMs / 1000).toStringAsFixed(1)}s',
                      'Stable',
                      true,
                    ),
                  ),
                  const SizedBox(width: 24),
                  Expanded(
                    child: _buildStatCard(
                      'Success Rate',
                      Icons.verified,
                      '${(stats.processingSuccessRate * 100).toStringAsFixed(1)}%',
                      'Current',
                      true,
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 40),
              Container(
                decoration: BoxDecoration(
                  color: TailwindColors.surfaceContainerLow,
                  borderRadius: BorderRadius.circular(12),
                  boxShadow: [
                    BoxShadow(
                      color: Colors.black.withValues(alpha: 0.02),
                      blurRadius: 4,
                      offset: const Offset(0, 2),
                    ),
                  ],
                ),
                child: Column(
                  children: [
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 24,
                        vertical: 16,
                      ),
                      decoration: const BoxDecoration(
                        color: TailwindColors.surfaceContainerLowest,
                        borderRadius: BorderRadius.vertical(
                          top: Radius.circular(12),
                        ),
                      ),
                      child: Row(
                        mainAxisAlignment: MainAxisAlignment.spaceBetween,
                        children: [
                          const Text(
                            'Processing History',
                            style: TextStyle(
                              fontWeight: FontWeight.bold,
                              fontSize: 18,
                              color: TailwindColors.onSurface,
                            ),
                          ),
                          Row(
                            children: [
                              OutlinedButton(
                                onPressed: _exportCsv,
                                style: OutlinedButton.styleFrom(
                                  foregroundColor:
                                      TailwindColors.onSurfaceVariant,
                                  side: const BorderSide(
                                    color: TailwindColors.outlineVariant,
                                  ),
                                  shape: RoundedRectangleBorder(
                                    borderRadius: BorderRadius.circular(8),
                                  ),
                                ),
                                child: const Text(
                                  'Export CSV',
                                  style: TextStyle(
                                    fontSize: 12,
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                              ),
                              const SizedBox(width: 8),
                              ElevatedButton(
                                onPressed: _loadData,
                                style: ElevatedButton.styleFrom(
                                  backgroundColor: TailwindColors.primary,
                                  foregroundColor: TailwindColors.onPrimary,
                                  shape: RoundedRectangleBorder(
                                    borderRadius: BorderRadius.circular(8),
                                  ),
                                ),
                                child: const Text(
                                  'Refresh',
                                  style: TextStyle(
                                    fontSize: 12,
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                    Container(
                      color: TailwindColors.surfaceContainer,
                      height: 1,
                    ),
                    Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 24,
                        vertical: 12,
                      ),
                      child: Row(
                        children: [
                          Expanded(
                            flex: 3,
                            child: Text(
                              'DOCUMENT NAME',
                              style: _tableHeaderStyle(),
                            ),
                          ),
                          Expanded(
                            flex: 4,
                            child: Text('STEPS', style: _tableHeaderStyle()),
                          ),
                          Expanded(
                            flex: 2,
                            child: Text('STATUS', style: _tableHeaderStyle()),
                          ),
                          Expanded(
                            flex: 2,
                            child: Text(
                              'TIMESTAMP',
                              style: _tableHeaderStyle(),
                            ),
                          ),
                          Expanded(
                            flex: 1,
                            child: Align(
                              alignment: Alignment.centerRight,
                              child: Text(
                                'PROC. TIME',
                                style: _tableHeaderStyle(),
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                    const Padding(
                      padding: EdgeInsets.fromLTRB(24, 0, 24, 12),
                      child: QueueStageLegend(),
                    ),
                    if (stats.recentRuns.isEmpty)
                      const Padding(
                        padding: EdgeInsets.all(32.0),
                        child: Text(
                          'No recent processing runs.',
                          style: TextStyle(
                            color: TailwindColors.onSurfaceVariant,
                          ),
                        ),
                      ),
                    ...stats.recentRuns.asMap().entries.map((entry) {
                      final idx = entry.key;
                      final item = entry.value;
                      return _buildTableRow(item, idx % 2 != 0);
                    }),
                  ],
                ),
              ),
            ],
          ),
        ),
        Positioned.fill(
          child: ProcessResultDrawer(
            item: _selectedResultItem,
            isOpen: _isResultDrawerOpen,
            onClose: _closeResultDrawer,
            onApply: _selectedResultItem != null
                ? () => _applyItem(_selectedResultItem!)
                : null,
            isApplying:
                _selectedResultItem != null &&
                _applyingItemId == _selectedResultItem!.id,
            showApplySuggestions:
                _selectedResultItem?.canApplySuggestions(
                  isManualMode: _processingMode == 'manual',
                  hasCompletedTag: _hasCompletedTag,
                ) ??
                false,
          ),
        ),
      ],
    );
  }

  TextStyle _tableHeaderStyle() {
    return const TextStyle(
      fontSize: 10,
      fontWeight: FontWeight.bold,
      color: TailwindColors.onSurfaceVariant,
      letterSpacing: 1.0,
    );
  }

  Widget _buildStatCard(
    String title,
    IconData icon,
    String value,
    String subText,
    bool positive,
  ) {
    return Container(
      height: 128,
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(
                title.toUpperCase(),
                style: const TextStyle(
                  fontSize: 10,
                  fontWeight: FontWeight.bold,
                  color: TailwindColors.onSurfaceVariant,
                  letterSpacing: 1.0,
                ),
              ),
              Icon(icon, color: TailwindColors.primaryContainer, size: 20),
            ],
          ),
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            crossAxisAlignment: CrossAxisAlignment.end,
            children: [
              Text(
                value,
                style: const TextStyle(
                  fontSize: 36,
                  fontWeight: FontWeight.bold,
                  color: TailwindColors.onSurface,
                  height: 1,
                ),
              ),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                decoration: BoxDecoration(
                  color: positive
                      ? TailwindColors.tertiary.withValues(alpha: 0.1)
                      : TailwindColors.error.withValues(alpha: 0.1),
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Row(
                  children: [
                    Icon(
                      positive ? Icons.trending_up : Icons.remove,
                      size: 14,
                      color: positive
                          ? TailwindColors.tertiary
                          : TailwindColors.error,
                    ),
                    const SizedBox(width: 4),
                    Text(
                      subText,
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.bold,
                        color: positive
                            ? TailwindColors.tertiary
                            : TailwindColors.error,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }

  Widget _buildTableRow(QueueItem item, bool alternate) {
    final canOpenDetails = item.canViewResultDetails;
    Color statusColor;
    switch (item.status) {
      case 'completed':
        statusColor = TailwindColors.tertiary;
        break;
      case 'partially_completed':
        statusColor = TailwindColors.secondary;
        break;
      case 'failed':
        statusColor = TailwindColors.error;
        break;
      case 'processing':
        statusColor = TailwindColors.primaryContainer;
        break;
      default:
        statusColor = TailwindColors.outline;
    }

    final dateStr = DateFormat(
      'MMM d, yyyy',
    ).format(DateTime.fromMillisecondsSinceEpoch(item.requestedAtMs));
    final timeStr = DateFormat(
      'HH:mm:ss',
    ).format(DateTime.fromMillisecondsSinceEpoch(item.requestedAtMs));
    final procTimeStr = item.processingDurationMs != null
        ? '${(item.processingDurationMs! / 1000).toStringAsFixed(2)}s'
        : '--';

    final row = Container(
      color: alternate ? TailwindColors.surface : Colors.transparent,
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 16),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            flex: 3,
            child: Row(
              children: [
                Container(
                  width: 32,
                  height: 32,
                  decoration: BoxDecoration(
                    color: TailwindColors.primary.withValues(alpha: 0.1),
                    borderRadius: BorderRadius.circular(4),
                  ),
                  child: const Icon(
                    Icons.description,
                    size: 16,
                    color: TailwindColors.primary,
                  ),
                ),
                Expanded(
                  child: Padding(
                    padding: const EdgeInsets.only(left: 12),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          item.documentTitle,
                          style: const TextStyle(
                            fontSize: 14,
                            fontWeight: FontWeight.bold,
                            color: TailwindColors.onSurface,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                        Text(
                          'Source: ${item.source}',
                          style: const TextStyle(
                            fontSize: 10,
                            color: TailwindColors.onSurfaceVariant,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
          Expanded(
            flex: 4,
            child: QueueStageGrid(
              item: item,
              configuredStageKeys: _configuredStageKeys,
            ),
          ),
          const SizedBox(width: 16),
          Expanded(
            flex: 2,
            child: Row(
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    color: statusColor,
                    shape: BoxShape.circle,
                  ),
                ),
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    item.status.toUpperCase(),
                    style: const TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w500,
                      color: TailwindColors.onSurface,
                    ),
                  ),
                ),
              ],
            ),
          ),
          Expanded(
            flex: 2,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  dateStr,
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color: TailwindColors.onSurface,
                  ),
                ),
                Text(
                  timeStr,
                  style: const TextStyle(
                    fontSize: 10,
                    color: TailwindColors.onSurfaceVariant,
                  ),
                ),
              ],
            ),
          ),
          Expanded(
            flex: 1,
            child: Align(
              alignment: Alignment.centerRight,
              child: Text(
                procTimeStr,
                style: const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.bold,
                  fontFamily: 'monospace',
                  color: TailwindColors.primary,
                ),
              ),
            ),
          ),
        ],
      ),
    );

    if (!canOpenDetails) {
      return row;
    }

    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: Material(
        color: Colors.transparent,
        child: InkWell(
          onTap: () => _openResultDrawer(item),
          hoverColor: TailwindColors.primary.withValues(alpha: 0.04),
          child: row,
        ),
      ),
    );
  }

  String _csvField(String value) {
    return '"${value.replaceAll('"', '""')}"';
  }
}
