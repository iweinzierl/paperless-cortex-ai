import 'dart:async';

import 'package:flutter/material.dart';
import 'package:intl/intl.dart';
import 'package:provider/provider.dart';
import 'package:webapp/models/models.dart';
import 'package:webapp/services/api_service.dart';
import 'package:webapp/theme.dart';
import 'package:webapp/widgets/process_result_drawer.dart';
import 'package:webapp/widgets/queue_stage_overview.dart';

class DocumentHistoryScreen extends StatefulWidget {
  final int documentId;

  const DocumentHistoryScreen({super.key, required this.documentId});

  @override
  State<DocumentHistoryScreen> createState() => _DocumentHistoryScreenState();
}

class _DocumentHistoryScreenState extends State<DocumentHistoryScreen> {
  bool _isLoading = true;
  String? _error;
  DocumentProcessOverview? _overview;
  Set<String> _configuredStageKeys = <String>{'extract_text'};
  String _processingMode = 'manual';
  bool _hasCompletedTag = false;
  int? _applyingItemId;
  QueueItem? _selectedResultItem;
  bool _isResultDrawerOpen = false;
  Timer? _drawerCleanupTimer;

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
        api.getDocumentProcessOverview(widget.documentId),
        api.getConfig(),
      ]);
      final overview = responses[0] as DocumentProcessOverview;
      final config = responses[1] as BackendConfig;

      if (!mounted) {
        return;
      }

      setState(() {
        _overview = overview;
        _configuredStageKeys = configuredQueueStageKeys(config.process);
        _processingMode = config.engine.processingMode;
        _hasCompletedTag = config.process.processCompletedTag.trim().isNotEmpty;
        _selectedResultItem = _refreshSelectedItem(
          overview.items,
          _selectedResultItem,
        );
        _isLoading = false;
      });
    } catch (e) {
      if (!mounted) {
        return;
      }
      final message = e is ApiException ? e.message : e.toString();
      setState(() {
        _error = 'Failed to load document history: $message';
        _isLoading = false;
      });
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
        if (_selectedResultItem?.id == updatedItem.id) {
          _selectedResultItem = updatedItem;
        }
        _applyingItemId = null;
      });
      await _loadData();
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            updatedItem.appliedSummary?.trim().isNotEmpty == true
                ? updatedItem.appliedSummary!
                : 'Suggestions applied successfully.',
          ),
        ),
      );
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
            FilledButton(onPressed: _loadData, child: const Text('Retry')),
          ],
        ),
      );
    }

    final overview = _overview!;
    final extractionSummary = _buildExtractionSummary(overview.items);

    return Stack(
      children: [
        SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 32, vertical: 28),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _HistoryHeader(
                title: overview.documentTitle,
                documentId: overview.documentId,
                runCount: overview.totalRuns,
                onRefresh: _loadData,
              ),
              const SizedBox(height: 24),
              Wrap(
                spacing: 16,
                runSpacing: 16,
                children: [
                  _MetricCard(
                    label: 'Completed Runs',
                    value: overview.completedRuns.toString(),
                    accent: TailwindColors.tertiary,
                  ),
                  _MetricCard(
                    label: 'Partial Runs',
                    value: overview.partiallyCompletedRuns.toString(),
                    accent: TailwindColors.secondary,
                  ),
                  _MetricCard(
                    label: 'Failed Runs',
                    value: overview.failedRuns.toString(),
                    accent: TailwindColors.error,
                  ),
                  _MetricCard(
                    label: 'Active Runs',
                    value: overview.activeRuns.toString(),
                    accent: TailwindColors.primary,
                  ),
                ],
              ),
              const SizedBox(height: 24),
              _SectionPanel(
                title: 'Extraction Focus',
                subtitle:
                    'Extraction source and model are surfaced before downstream classification so differences between OCR strategies are easier to compare.',
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Wrap(
                      spacing: 12,
                      runSpacing: 12,
                      children: extractionSummary
                          .map(
                            (entry) => _SummaryPill(
                              label: entry.label,
                              value: entry.value,
                            ),
                          )
                          .toList(growable: false),
                    ),
                    if (overview.distinctLlms.isNotEmpty) ...[
                      const SizedBox(height: 16),
                      _BadgeRow(
                        title: 'LLMs',
                        values: overview.distinctLlms,
                        backgroundColor: TailwindColors.primaryFixed,
                        foregroundColor: TailwindColors.onPrimaryFixedVariant,
                      ),
                    ],
                    if (overview.distinctVisionLlms.isNotEmpty) ...[
                      const SizedBox(height: 12),
                      _BadgeRow(
                        title: 'Vision Models',
                        values: overview.distinctVisionLlms,
                        backgroundColor: TailwindColors.secondaryContainer,
                        foregroundColor: TailwindColors.onSecondaryContainer,
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(height: 24),
              _SectionPanel(
                title: 'Run Comparison',
                subtitle:
                    'Each run shows the extraction mode first, then the concrete result of each requested step.',
                child: Column(
                  children: [
                    const Padding(
                      padding: EdgeInsets.only(bottom: 16),
                      child: QueueStageLegend(),
                    ),
                    ...overview.items.map(
                      (item) => Padding(
                        padding: const EdgeInsets.only(bottom: 18),
                        child: _RunComparisonCard(
                          item: item,
                          configuredStageKeys: _configuredStageKeys,
                          onOpenDetails: _openResultDrawer,
                        ),
                      ),
                    ),
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

  List<_SummaryEntry> _buildExtractionSummary(List<QueueItem> items) {
    final sourceCounts = <String, int>{};
    final modelCounts = <String, int>{};

    for (final item in items) {
      final extraction = item.processingResult?.extraction;
      final source = extraction?.source.trim() ?? '';
      if (source.isNotEmpty) {
        sourceCounts[source] = (sourceCounts[source] ?? 0) + 1;
      }
      final model = extraction?.usedModel.trim() ?? '';
      if (model.isNotEmpty) {
        modelCounts[model] = (modelCounts[model] ?? 0) + 1;
      }
    }

    final entries = <_SummaryEntry>[];
    for (final entry in sourceCounts.entries) {
      entries.add(_SummaryEntry('Mode', '${entry.key} (${entry.value})'));
    }
    for (final entry in modelCounts.entries) {
      entries.add(_SummaryEntry('Extractor', '${entry.key} (${entry.value})'));
    }

    if (entries.isEmpty) {
      entries.add(const _SummaryEntry('Mode', 'No extraction results yet'));
    }

    return entries;
  }
}

class _HistoryHeader extends StatelessWidget {
  final String title;
  final int documentId;
  final int runCount;
  final VoidCallback onRefresh;

  const _HistoryHeader({
    required this.title,
    required this.documentId,
    required this.runCount,
    required this.onRefresh,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Text(
                'Document History',
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 1.1,
                  color: TailwindColors.primary,
                ),
              ),
              const SizedBox(height: 8),
              Text(
                title,
                style: const TextStyle(
                  fontSize: 30,
                  fontWeight: FontWeight.w800,
                  color: TailwindColors.onSurface,
                  height: 1.1,
                ),
              ),
              const SizedBox(height: 10),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  _Badge(text: 'Document #$documentId'),
                  _Badge(text: '$runCount stored runs'),
                ],
              ),
            ],
          ),
        ),
        FilledButton.icon(
          onPressed: onRefresh,
          icon: const Icon(Icons.refresh, size: 18),
          label: const Text('Refresh'),
        ),
      ],
    );
  }
}

class _MetricCard extends StatelessWidget {
  final String label;
  final String value;
  final Color accent;

  const _MetricCard({
    required this.label,
    required this.value,
    required this.accent,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 220,
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerLow,
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: TailwindColors.surfaceContainerHigh),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Container(
            width: 36,
            height: 4,
            decoration: BoxDecoration(
              color: accent,
              borderRadius: BorderRadius.circular(999),
            ),
          ),
          const SizedBox(height: 16),
          Text(
            value,
            style: const TextStyle(
              fontSize: 28,
              fontWeight: FontWeight.w800,
              color: TailwindColors.onSurface,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            label,
            style: const TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w700,
              color: TailwindColors.onSurfaceVariant,
            ),
          ),
        ],
      ),
    );
  }
}

class _SectionPanel extends StatelessWidget {
  final String title;
  final String subtitle;
  final Widget child;

  const _SectionPanel({
    required this.title,
    required this.subtitle,
    required this.child,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(22),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerLowest,
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: TailwindColors.surfaceContainerHigh),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w800,
              color: TailwindColors.onSurface,
            ),
          ),
          const SizedBox(height: 6),
          Text(
            subtitle,
            style: const TextStyle(
              fontSize: 13,
              height: 1.45,
              color: TailwindColors.onSurfaceVariant,
            ),
          ),
          const SizedBox(height: 18),
          child,
        ],
      ),
    );
  }
}

class _RunComparisonCard extends StatelessWidget {
  final QueueItem item;
  final Set<String> configuredStageKeys;
  final ValueChanged<QueueItem> onOpenDetails;

  const _RunComparisonCard({
    required this.item,
    required this.configuredStageKeys,
    required this.onOpenDetails,
  });

  @override
  Widget build(BuildContext context) {
    final result = item.processingResult;
    final extraction = result?.extraction;

    return Container(
      padding: const EdgeInsets.all(20),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerLow,
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: TailwindColors.surfaceContainerHigh),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Wrap(
                  spacing: 8,
                  runSpacing: 8,
                  children: [
                    _StatusBadge(status: item.status),
                    _Badge(text: 'Queue #${item.id}'),
                    _Badge(text: _formatTimestamp(item.requestedAt)),
                    if (item.processingDurationMs != null)
                      _Badge(text: _formatDuration(item.processingDurationMs)),
                  ],
                ),
              ),
              TextButton.icon(
                onPressed: item.canViewResultDetails
                    ? () => onOpenDetails(item)
                    : null,
                icon: const Icon(Icons.visibility_outlined, size: 18),
                label: const Text('Run Details'),
              ),
            ],
          ),
          const SizedBox(height: 16),
          QueueStageGrid(
            item: item,
            configuredStageKeys: configuredStageKeys,
            minColumnWidth: 80,
          ),
          const SizedBox(height: 18),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: TailwindColors.surface,
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: TailwindColors.surfaceContainerHigh),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text(
                  'Extraction',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w800,
                    color: TailwindColors.onSurface,
                  ),
                ),
                const SizedBox(height: 10),
                Wrap(
                  spacing: 10,
                  runSpacing: 10,
                  children: [
                    _SummaryPill(
                      label: 'Mode',
                      value: extraction?.source.trim().isNotEmpty == true
                          ? extraction!.source
                          : 'Unknown',
                    ),
                    _SummaryPill(
                      label: 'Model',
                      value: extraction?.usedModel.trim().isNotEmpty == true
                          ? extraction!.usedModel
                          : '--',
                    ),
                    _SummaryPill(
                      label: 'Text Length',
                      value: extraction != null
                          ? extraction.textLength.toString()
                          : '--',
                    ),
                    _SummaryPill(
                      label: 'Status',
                      value: extraction?.status ?? 'skipped',
                    ),
                  ],
                ),
                if (extraction?.error.trim().isNotEmpty == true) ...[
                  const SizedBox(height: 12),
                  Text(
                    extraction!.error,
                    style: const TextStyle(
                      fontSize: 12,
                      color: TailwindColors.error,
                    ),
                  ),
                ],
                if (extraction?.textPreview.trim().isNotEmpty == true) ...[
                  const SizedBox(height: 12),
                  Text(
                    extraction!.textPreview,
                    maxLines: 5,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(
                      fontSize: 12,
                      height: 1.5,
                      color: TailwindColors.onSurfaceVariant,
                    ),
                  ),
                ],
              ],
            ),
          ),
          const SizedBox(height: 16),
          const Text(
            'Step Results',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w800,
              color: TailwindColors.onSurface,
            ),
          ),
          const SizedBox(height: 12),
          Wrap(
            spacing: 12,
            runSpacing: 12,
            children: _buildStageResultEntries(item)
                .map((entry) => _StageResultCard(entry: entry))
                .toList(growable: false),
          ),
        ],
      ),
    );
  }

  List<_StageResultEntry> _buildStageResultEntries(QueueItem item) {
    final result = item.processingResult;
    if (result == null) {
      return const [];
    }

    return [
      _StageResultEntry(
        label: 'Created Date',
        status: result.createdDate.status,
        value: _payloadString(result.createdDate.payload, 'created') ?? '--',
        confidence: result.createdDate.confidence,
        detail: result.createdDate.error.isNotEmpty
            ? result.createdDate.error
            : result.createdDate.reasoning,
      ),
      _StageResultEntry(
        label: 'Correspondent',
        status: result.correspondent.status,
        value:
            _payloadString(
              result.correspondent.payload,
              'correspondent_name',
            ) ??
            _payloadString(
              result.correspondent.payload,
              'suggested_new_correspondent',
            ) ??
            '--',
        confidence: result.correspondent.confidence,
        detail: result.correspondent.error.isNotEmpty
            ? result.correspondent.error
            : result.correspondent.reasoning,
      ),
      _StageResultEntry(
        label: 'Document Type',
        status: result.documentType.status,
        value:
            _payloadString(result.documentType.payload, 'document_type_name') ??
            _payloadString(
              result.documentType.payload,
              'suggested_new_document_type',
            ) ??
            '--',
        confidence: result.documentType.confidence,
        detail: result.documentType.error.isNotEmpty
            ? result.documentType.error
            : result.documentType.reasoning,
      ),
      _StageResultEntry(
        label: 'Tags',
        status: result.tags.status,
        value: _joinTagValues(result.tags.payload),
        confidence: result.tags.confidence,
        detail: result.tags.error.isNotEmpty
            ? result.tags.error
            : result.tags.reasoning,
      ),
      _StageResultEntry(
        label: 'Title',
        status: result.title.status,
        value: _payloadString(result.title.payload, 'title') ?? '--',
        confidence: result.title.confidence,
        detail: result.title.error.isNotEmpty
            ? result.title.error
            : result.title.reasoning,
      ),
    ];
  }

  static String _joinTagValues(Map<String, dynamic>? payload) {
    final tagNames = _payloadStringList(payload, 'tag_names');
    final suggested = _payloadStringList(payload, 'suggested_new_tags');
    final values = [...tagNames, ...suggested];
    return values.isEmpty ? '--' : values.join(', ');
  }

  static String? _payloadString(Map<String, dynamic>? payload, String key) {
    final value = payload?[key];
    if (value == null) {
      return null;
    }
    final text = value.toString().trim();
    return text.isEmpty ? null : text;
  }

  static List<String> _payloadStringList(
    Map<String, dynamic>? payload,
    String key,
  ) {
    final value = payload?[key];
    if (value is! List) {
      return const [];
    }
    return value
        .map((entry) => entry.toString().trim())
        .where((entry) => entry.isNotEmpty)
        .toList(growable: false);
  }

  static String _formatTimestamp(DateTime value) {
    return DateFormat('MMM d, yyyy HH:mm:ss').format(value);
  }

  static String _formatDuration(int? durationMs) {
    if (durationMs == null || durationMs <= 0) {
      return '--';
    }
    final duration = Duration(milliseconds: durationMs);
    if (duration.inHours > 0) {
      return '${duration.inHours}h ${duration.inMinutes.remainder(60)}m ${duration.inSeconds.remainder(60)}s';
    }
    if (duration.inMinutes > 0) {
      return '${duration.inMinutes}m ${duration.inSeconds.remainder(60)}s';
    }
    return '${(durationMs / 1000).toStringAsFixed(2)}s';
  }
}

class _StageResultEntry {
  final String label;
  final String status;
  final String value;
  final String confidence;
  final String detail;

  const _StageResultEntry({
    required this.label,
    required this.status,
    required this.value,
    required this.confidence,
    required this.detail,
  });
}

class _StageResultCard extends StatelessWidget {
  final _StageResultEntry entry;

  const _StageResultCard({required this.entry});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 228,
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: TailwindColors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: TailwindColors.surfaceContainerHigh),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  entry.label,
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w800,
                    color: TailwindColors.onSurface,
                  ),
                ),
              ),
              _TinyStatusBadge(status: entry.status),
            ],
          ),
          const SizedBox(height: 10),
          Text(
            entry.value,
            maxLines: 4,
            overflow: TextOverflow.ellipsis,
            style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: TailwindColors.onSurface,
            ),
          ),
          if (entry.confidence.trim().isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              'Confidence: ${entry.confidence}',
              style: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w700,
                color: TailwindColors.primary,
              ),
            ),
          ],
          if (entry.detail.trim().isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              entry.detail,
              maxLines: 4,
              overflow: TextOverflow.ellipsis,
              style: const TextStyle(
                fontSize: 11,
                height: 1.45,
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ],
        ],
      ),
    );
  }
}

class _Badge extends StatelessWidget {
  final String text;

  const _Badge({required this.text});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        text,
        style: const TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w700,
          color: TailwindColors.onSurfaceVariant,
        ),
      ),
    );
  }
}

class _StatusBadge extends StatelessWidget {
  final String status;

  const _StatusBadge({required this.status});

  @override
  Widget build(BuildContext context) {
    final colors = _statusColors(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
      decoration: BoxDecoration(
        color: colors.background,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        status.replaceAll('_', ' ').toUpperCase(),
        style: TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w800,
          color: colors.foreground,
        ),
      ),
    );
  }
}

class _TinyStatusBadge extends StatelessWidget {
  final String status;

  const _TinyStatusBadge({required this.status});

  @override
  Widget build(BuildContext context) {
    final colors = _statusColors(status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: colors.background,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        status.toUpperCase(),
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.w800,
          color: colors.foreground,
        ),
      ),
    );
  }
}

class _BadgeRow extends StatelessWidget {
  final String title;
  final List<String> values;
  final Color backgroundColor;
  final Color foregroundColor;

  const _BadgeRow({
    required this.title,
    required this.values,
    required this.backgroundColor,
    required this.foregroundColor,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 110,
          child: Padding(
            padding: const EdgeInsets.only(top: 6),
            child: Text(
              title,
              style: const TextStyle(
                fontSize: 12,
                fontWeight: FontWeight.w700,
                color: TailwindColors.onSurface,
              ),
            ),
          ),
        ),
        Expanded(
          child: Wrap(
            spacing: 8,
            runSpacing: 8,
            children: values
                .map(
                  (value) => Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 10,
                      vertical: 6,
                    ),
                    decoration: BoxDecoration(
                      color: backgroundColor,
                      borderRadius: BorderRadius.circular(999),
                    ),
                    child: Text(
                      value,
                      style: TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                        color: foregroundColor,
                      ),
                    ),
                  ),
                )
                .toList(growable: false),
          ),
        ),
      ],
    );
  }
}

class _SummaryPill extends StatelessWidget {
  final String label;
  final String value;

  const _SummaryPill({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      decoration: BoxDecoration(
        color: TailwindColors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: TailwindColors.surfaceContainerHigh),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label,
            style: const TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.w800,
              color: TailwindColors.onSurfaceVariant,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            value,
            style: const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: TailwindColors.onSurface,
            ),
          ),
        ],
      ),
    );
  }
}

class _SummaryEntry {
  final String label;
  final String value;

  const _SummaryEntry(this.label, this.value);
}

({Color background, Color foreground}) _statusColors(String status) {
  switch (status) {
    case 'completed':
      return (
        background: TailwindColors.tertiaryFixed,
        foreground: TailwindColors.tertiary,
      );
    case 'partially_completed':
      return (
        background: TailwindColors.secondaryContainer,
        foreground: TailwindColors.onSecondaryContainer,
      );
    case 'failed':
      return (
        background: TailwindColors.errorContainer,
        foreground: TailwindColors.error,
      );
    case 'running':
    case 'processing':
      return (
        background: TailwindColors.primaryContainer,
        foreground: TailwindColors.primary,
      );
    default:
      return (
        background: TailwindColors.surfaceContainerHighest,
        foreground: TailwindColors.onSurfaceVariant,
      );
  }
}
