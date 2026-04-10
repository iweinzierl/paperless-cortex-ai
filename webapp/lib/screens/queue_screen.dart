import 'dart:async';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:webapp/theme.dart';
import 'package:webapp/services/api_service.dart';
import 'package:webapp/models/models.dart';
import 'package:webapp/widgets/process_result_drawer.dart';
import 'package:intl/intl.dart';

class QueueScreen extends StatefulWidget {
  const QueueScreen({super.key});

  @override
  State<QueueScreen> createState() => _QueueScreenState();
}

class _QueueScreenState extends State<QueueScreen> {
  static const Duration _pollInterval = Duration(seconds: 3);
  static const int _pageSize = 25;

  bool _isLoading = true;
  bool _isFetching = false;
  String? _error;
  List<QueueItem> _items = [];
  List<QueueItem> _activeItems = [];
  String _filterStatus = 'all';
  String _searchQuery = '';
  int _currentPage = 0;
  Timer? _pollTimer;
  Timer? _drawerCleanupTimer;
  QueueItem? _selectedResultItem;
  bool _isResultDrawerOpen = false;
  int? _removingItemId;
  final TextEditingController _searchController = TextEditingController();

  @override
  void initState() {
    super.initState();
    _loadData();
  }

  @override
  void dispose() {
    _searchController.dispose();
    _pollTimer?.cancel();
    _drawerCleanupTimer?.cancel();
    super.dispose();
  }

  Future<void> _loadData({bool showLoadingState = true}) async {
    if (_isFetching) {
      return;
    }

    _isFetching = true;
    if (showLoadingState) {
      setState(() {
        _isLoading = true;
        _error = null;
      });
    }

    try {
      final api = context.read<ApiService>();
      final statusQuery = _filterStatus == 'all' ? null : _filterStatus;
      final responses = await Future.wait([
        api.getQueue(status: statusQuery, limit: 100),
        api.getQueue(status: 'processing', limit: 20),
      ]);
      final items = responses[0];
      final activeItems = responses[1];
      if (mounted) {
        setState(() {
          _items = items;
          _activeItems = activeItems;
          _selectedResultItem = _refreshSelectedItem(
            items,
            _selectedResultItem,
          );
          _isLoading = false;
          _error = null;
        });
        _syncPolling();
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Failed to load queue: $e';
          _isLoading = false;
        });
        _pollTimer?.cancel();
        _pollTimer = null;
      }
    } finally {
      _isFetching = false;
    }
  }

  Future<void> _processItem(int id) async {
    try {
      final api = context.read<ApiService>();
      final triggeredItem = await api.processQueueItem(id);

      if (!mounted) {
        return;
      }

      setState(() {
        _items = _upsertQueueItem(_items, triggeredItem);
        _activeItems = _upsertQueueItem(_activeItems, triggeredItem);
      });
      _syncPolling();
      await _loadData(showLoadingState: false);
    } catch (e) {
      if (!mounted) {
        return;
      }
      final errorMessage = e is ApiException ? e.message : e.toString();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text('Processing failed: $errorMessage')),
      );
    }
  }

  Future<void> _removeItem(QueueItem item) async {
    if (_removingItemId != null) {
      return;
    }

    final confirmed = await showDialog<bool>(
      context: context,
      builder: (dialogContext) {
        return AlertDialog(
          title: const Text('Remove queue item?'),
          content: Text(
            'This removes "${item.documentTitle}" from the queue history. Processing items cannot be removed while they are running.',
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(dialogContext).pop(false),
              child: const Text('Cancel'),
            ),
            FilledButton(
              onPressed: () => Navigator.of(dialogContext).pop(true),
              style: FilledButton.styleFrom(
                backgroundColor: TailwindColors.error,
                foregroundColor: TailwindColors.onError,
              ),
              child: const Text('Remove'),
            ),
          ],
        );
      },
    );

    if (confirmed != true || !mounted) {
      return;
    }

    setState(() {
      _removingItemId = item.id;
    });

    try {
      final api = context.read<ApiService>();
      await api.deleteQueueItem(item.id);

      if (!mounted) {
        return;
      }

      _drawerCleanupTimer?.cancel();
      setState(() {
        _items = _removeQueueItem(_items, item.id);
        _activeItems = _removeQueueItem(_activeItems, item.id);
        if (_selectedResultItem?.id == item.id) {
          _selectedResultItem = null;
          _isResultDrawerOpen = false;
        }
        _removingItemId = null;
      });
      _syncPolling();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text('Removed "${item.documentTitle}" from the queue.'),
        ),
      );
    } catch (e) {
      if (!mounted) {
        return;
      }
      setState(() {
        _removingItemId = null;
      });
      final errorMessage = e is ApiException ? e.message : e.toString();
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('Remove failed: $errorMessage')));
    }
  }

  void _syncPolling() {
    final shouldPoll = _activeItems.isNotEmpty;
    if (!shouldPoll) {
      _pollTimer?.cancel();
      _pollTimer = null;
      return;
    }

    if (_pollTimer != null && _pollTimer!.isActive) {
      return;
    }

    _pollTimer = Timer.periodic(_pollInterval, (_) {
      _loadData(showLoadingState: false);
    });
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

  List<QueueItem> _upsertQueueItem(List<QueueItem> items, QueueItem item) {
    final updated = List<QueueItem>.from(items);
    final existingIndex = updated.indexWhere((entry) => entry.id == item.id);
    if (existingIndex >= 0) {
      updated[existingIndex] = item;
    } else {
      updated.insert(0, item);
    }

    if (item.status != 'processing') {
      updated.removeWhere(
        (entry) => entry.id == item.id && entry.status != item.status,
      );
    }

    return updated;
  }

  List<QueueItem> _removeQueueItem(List<QueueItem> items, int id) {
    return items.where((entry) => entry.id != id).toList();
  }

  List<QueueItem> _filteredItems(List<QueueItem> items) {
    final query = _searchQuery.trim().toLowerCase();
    if (query.isEmpty) {
      return items;
    }

    return items
        .where((item) {
          final haystack = [
            item.documentTitle,
            item.source,
            item.status,
            item.documentId?.toString() ?? '',
            item.resultSummary ?? '',
          ].join(' ').toLowerCase();
          return haystack.contains(query);
        })
        .toList(growable: false);
  }

  int _pageCount(List<QueueItem> items) {
    if (items.isEmpty) {
      return 1;
    }
    return ((items.length - 1) / _pageSize).floor() + 1;
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

    int pendingCount = _items.where((i) => i.status == 'pending').length;
    int errorCount = _items
        .where((i) => i.status == 'failed' || i.status == 'partially_completed')
        .length;
    int processingCount = _activeItems.length;
    final filteredItems = _filteredItems(_items);
    final totalPages = _pageCount(filteredItems);
    final currentPage = totalPages == 0
        ? 0
        : _currentPage.clamp(0, totalPages - 1);
    final startIndex = filteredItems.isEmpty ? 0 : currentPage * _pageSize;
    final visibleItems = filteredItems
        .skip(startIndex)
        .take(_pageSize)
        .toList(growable: false);
    final endIndex = filteredItems.isEmpty
        ? 0
        : startIndex + visibleItems.length;

    return Stack(
      children: [
        SingleChildScrollView(
          padding: const EdgeInsets.symmetric(horizontal: 40.0, vertical: 32.0),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Header & Stats
              Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Text(
                        'Processing Queue',
                        style: TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w800,
                          color: TailwindColors.onSurface,
                          letterSpacing: -0.5,
                        ),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        '${_items.length} items currently in log. Pending processes execute asynchronously.',
                        style: const TextStyle(
                          color: TailwindColors.onSurfaceVariant,
                          fontSize: 14,
                        ),
                      ),
                    ],
                  ),
                  Row(
                    children: [
                      _buildTopStatCard(
                        'Queued',
                        pendingCount.toString(),
                        TailwindColors.surfaceContainerLowest,
                        null,
                      ),
                      const SizedBox(width: 16),
                      _buildTopStatCard(
                        'Processing',
                        processingCount.toString(),
                        TailwindColors.primaryFixed,
                        TailwindColors.primary,
                      ),
                      const SizedBox(width: 16),
                      _buildTopStatCard(
                        'Error State',
                        errorCount.toString(),
                        TailwindColors.errorContainer,
                        TailwindColors.error,
                      ),
                    ],
                  ),
                ],
              ),
              const SizedBox(height: 32),

              // Main Table Area
              Container(
                decoration: BoxDecoration(
                  color: TailwindColors.surfaceContainerLow,
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(
                    color: TailwindColors.outlineVariant.withValues(
                      alpha: 0.15,
                    ),
                  ),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    // Table Toolbar
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
                        children: [
                          // Filter Dropdown
                          Container(
                            padding: const EdgeInsets.symmetric(
                              horizontal: 16,
                              vertical: 8,
                            ),
                            decoration: BoxDecoration(
                              color: TailwindColors.surfaceContainerHighest,
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: DropdownButtonHideUnderline(
                              child: DropdownButton<String>(
                                value: _filterStatus,
                                isDense: true,
                                icon: const Icon(
                                  Icons.filter_list,
                                  size: 16,
                                  color: TailwindColors.onSurfaceVariant,
                                ),
                                style: const TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.bold,
                                  color: TailwindColors.onSurface,
                                ),
                                onChanged: (String? newValue) {
                                  if (newValue != null) {
                                    setState(() => _filterStatus = newValue);
                                    _loadData();
                                  }
                                },
                                items: const [
                                  DropdownMenuItem(
                                    value: 'all',
                                    child: Text('All Events'),
                                  ),
                                  DropdownMenuItem(
                                    value: 'pending',
                                    child: Text('Pending'),
                                  ),
                                  DropdownMenuItem(
                                    value: 'processing',
                                    child: Text('Processing'),
                                  ),
                                  DropdownMenuItem(
                                    value: 'completed',
                                    child: Text('Completed'),
                                  ),
                                  DropdownMenuItem(
                                    value: 'partially_completed',
                                    child: Text('Partially Completed'),
                                  ),
                                  DropdownMenuItem(
                                    value: 'failed',
                                    child: Text('Failed'),
                                  ),
                                ],
                              ),
                            ),
                          ),
                          const Spacer(),
                          // Search inside items
                          Container(
                            width: 240,
                            height: 36,
                            decoration: BoxDecoration(
                              color: TailwindColors.surfaceContainerHighest,
                              borderRadius: BorderRadius.circular(8),
                            ),
                            child: TextField(
                              controller: _searchController,
                              onChanged: (value) {
                                setState(() {
                                  _searchQuery = value;
                                  _currentPage = 0;
                                });
                              },
                              style: const TextStyle(fontSize: 12),
                              decoration: const InputDecoration(
                                hintText: 'Search queue entries...',
                                prefixIcon: Icon(
                                  Icons.search,
                                  size: 16,
                                  color: TailwindColors.outline,
                                ),
                                border: InputBorder.none,
                                contentPadding: EdgeInsets.symmetric(
                                  vertical: 12,
                                ),
                              ),
                            ),
                          ),
                          const SizedBox(width: 8),
                          OutlinedButton.icon(
                            onPressed: () => _loadData(),
                            icon: const Icon(Icons.refresh, size: 16),
                            label: const Text(
                              'Refresh',
                              style: TextStyle(
                                fontSize: 12,
                                fontWeight: FontWeight.bold,
                              ),
                            ),
                            style: OutlinedButton.styleFrom(
                              foregroundColor: TailwindColors.onSurfaceVariant,
                              side: const BorderSide(
                                color: TailwindColors.outlineVariant,
                              ),
                              shape: RoundedRectangleBorder(
                                borderRadius: BorderRadius.circular(8),
                              ),
                              padding: const EdgeInsets.symmetric(
                                horizontal: 16,
                                vertical: 12,
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                    Container(
                      color: TailwindColors.surfaceContainer,
                      height: 1,
                    ),

                    // Column Headers
                    Padding(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 24,
                        vertical: 12,
                      ),
                      child: Row(
                        children: [
                          Expanded(
                            flex: 3,
                            child: Text('DOCUMENT', style: _tableHeaderStyle()),
                          ),
                          Expanded(
                            flex: 2,
                            child: Text('STATUS', style: _tableHeaderStyle()),
                          ),
                          Expanded(
                            flex: 2,
                            child: Text('ADDED', style: _tableHeaderStyle()),
                          ),
                          Expanded(
                            flex: 2,
                            child: Text('RETRIES', style: _tableHeaderStyle()),
                          ),
                          const SizedBox(width: 120), // actions spacer
                        ],
                      ),
                    ),

                    // Rows
                    if (filteredItems.isEmpty)
                      const Padding(
                        padding: EdgeInsets.all(40),
                        child: Center(
                          child: Text(
                            'No queue entries match the current filters or search.',
                            style: TextStyle(
                              fontSize: 14,
                              color: TailwindColors.onSurfaceVariant,
                            ),
                          ),
                        ),
                      ),

                    ...visibleItems.map((item) => _buildQueueRow(item)),

                    // Pagination Footer
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 24,
                        vertical: 12,
                      ),
                      decoration: const BoxDecoration(
                        color: TailwindColors.surfaceContainerLowest,
                        borderRadius: BorderRadius.vertical(
                          bottom: Radius.circular(12),
                        ),
                      ),
                      child: Row(
                        mainAxisAlignment: MainAxisAlignment.spaceBetween,
                        children: [
                          Text(
                            filteredItems.isEmpty
                                ? 'No queue entries to display'
                                : 'Showing ${startIndex + 1}-$endIndex of ${filteredItems.length} queue entries',
                            style: const TextStyle(
                              fontSize: 12,
                              color: TailwindColors.outline,
                            ),
                          ),
                          Row(
                            children: [
                              IconButton(
                                onPressed: currentPage > 0
                                    ? () {
                                        setState(() {
                                          _currentPage = currentPage - 1;
                                        });
                                      }
                                    : null,
                                icon: const Icon(Icons.chevron_left, size: 20),
                                color: TailwindColors.onSurfaceVariant,
                                constraints: const BoxConstraints(),
                              ),
                              const SizedBox(width: 8),
                              Text(
                                'Page ${filteredItems.isEmpty ? 0 : currentPage + 1} of $totalPages',
                                style: const TextStyle(
                                  fontSize: 12,
                                  fontWeight: FontWeight.w600,
                                  color: TailwindColors.onSurface,
                                ),
                              ),
                              const SizedBox(width: 8),
                              IconButton(
                                onPressed: currentPage + 1 < totalPages
                                    ? () {
                                        setState(() {
                                          _currentPage = currentPage + 1;
                                        });
                                      }
                                    : null,
                                icon: const Icon(Icons.chevron_right, size: 20),
                                color: TailwindColors.onSurfaceVariant,
                                constraints: const BoxConstraints(),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              if (_activeItems.isNotEmpty) ...[
                const SizedBox(height: 24),
                _buildProgressPanel(),
              ],
            ],
          ),
        ),
        Positioned.fill(
          child: ProcessResultDrawer(
            item: _selectedResultItem,
            isOpen: _isResultDrawerOpen,
            onClose: _closeResultDrawer,
            onRemove: _selectedResultItem != null
                ? () => _removeItem(_selectedResultItem!)
                : null,
            isRemoving:
                _selectedResultItem != null &&
                _removingItemId == _selectedResultItem!.id,
          ),
        ),
      ],
    );
  }

  Widget _buildTopStatCard(
    String label,
    String count,
    Color bgColor,
    Color? fgColor,
  ) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 16),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: TailwindColors.outlineVariant.withValues(alpha: 0.15),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            label.toUpperCase(),
            style: const TextStyle(
              fontSize: 10,
              fontWeight: FontWeight.bold,
              color: TailwindColors.onSurfaceVariant,
              letterSpacing: 0.5,
            ),
          ),
          const SizedBox(height: 4),
          Text(
            count,
            style: TextStyle(
              fontSize: 24,
              fontWeight: FontWeight.w800,
              color: fgColor ?? TailwindColors.onSurface,
              height: 1,
            ),
          ),
        ],
      ),
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

  Widget _buildProgressPanel() {
    return Container(
      decoration: BoxDecoration(
        color: TailwindColors.inverseSurface,
        borderRadius: BorderRadius.circular(16),
        boxShadow: const [
          BoxShadow(
            color: Color(0x14000000),
            blurRadius: 24,
            offset: Offset(0, 12),
          ),
        ],
      ),
      padding: const EdgeInsets.all(24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(
                Icons.terminal,
                color: TailwindColors.primaryFixedDim,
                size: 18,
              ),
              const SizedBox(width: 8),
              const Text(
                'REAL-TIME PROCESS PROGRESS',
                style: TextStyle(
                  color: TailwindColors.primaryFixedDim,
                  fontSize: 11,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 1.2,
                ),
              ),
              const Spacer(),
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 6,
                ),
                decoration: BoxDecoration(
                  color: TailwindColors.tertiary.withValues(alpha: 0.18),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Text(
                  '${_activeItems.length} active',
                  style: const TextStyle(
                    color: TailwindColors.tertiaryFixed,
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 6),
          const Text(
            'Processing snapshots refresh automatically while at least one item is running.',
            style: TextStyle(
              color: TailwindColors.inverseOnSurface,
              fontSize: 13,
            ),
          ),
          const SizedBox(height: 20),
          ..._activeItems.map(_buildActiveProgressCard),
        ],
      ),
    );
  }

  Widget _buildActiveProgressCard(QueueItem item) {
    final processingResult = item.processingResult;
    final requestedStages =
        processingResult?.requestedStages ?? const <ProcessingStageProgress>[];
    final activeStage = processingResult?.activeStage;
    final progressValue =
        processingResult?.requestedStageCount != null &&
            (processingResult?.requestedStageCount ?? 0) > 0
        ? processingResult!.completionFraction
        : null;
    final elapsedLabel = _formatDuration(item.elapsedDuration);
    final statusLine = activeStage?.detail?.isNotEmpty == true
        ? activeStage!.detail!
        : (item.resultSummary?.isNotEmpty == true
              ? item.resultSummary!
              : 'Initializing processing pipeline.');

    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: TailwindColors.inverseOnSurface.withValues(alpha: 0.06),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(
          color: TailwindColors.primaryFixed.withValues(alpha: 0.18),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      item.documentTitle,
                      style: const TextStyle(
                        color: TailwindColors.surfaceContainerLowest,
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      activeStage != null
                          ? 'Current stage: ${activeStage.label}'
                          : 'Preparing workflow state',
                      style: const TextStyle(
                        color: TailwindColors.primaryFixed,
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 12),
              Column(
                crossAxisAlignment: CrossAxisAlignment.end,
                children: [
                  Text(
                    elapsedLabel,
                    style: const TextStyle(
                      color: TailwindColors.surfaceContainerLowest,
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    'Queue #${item.id}',
                    style: const TextStyle(
                      color: TailwindColors.primaryFixedDim,
                      fontSize: 11,
                    ),
                  ),
                ],
              ),
            ],
          ),
          const SizedBox(height: 12),
          ClipRRect(
            borderRadius: BorderRadius.circular(999),
            child: LinearProgressIndicator(
              minHeight: 7,
              value: progressValue,
              backgroundColor: TailwindColors.surfaceContainerHighest
                  .withValues(alpha: 0.45),
              valueColor: const AlwaysStoppedAnimation<Color>(
                TailwindColors.primaryFixedDim,
              ),
            ),
          ),
          const SizedBox(height: 12),
          Text(
            statusLine,
            style: const TextStyle(
              color: TailwindColors.inverseOnSurface,
              fontSize: 12,
              height: 1.4,
            ),
          ),
          if (requestedStages.isNotEmpty) ...[
            const SizedBox(height: 14),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: requestedStages.map(_buildStageChip).toList(),
            ),
          ],
          if ((item.usedLlm?.isNotEmpty == true) ||
              (item.usedVisionLlm?.isNotEmpty == true)) ...[
            const SizedBox(height: 12),
            Text(
              _buildModelSummary(item),
              style: const TextStyle(
                color: TailwindColors.primaryFixedDim,
                fontSize: 11,
              ),
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildStageChip(ProcessingStageProgress stage) {
    final colors = _stageColors(stage.status);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      decoration: BoxDecoration(
        color: colors.background,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(_stageIcon(stage.status), size: 14, color: colors.foreground),
          const SizedBox(width: 6),
          Text(
            stage.label,
            style: TextStyle(
              color: colors.foreground,
              fontSize: 11,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
  }

  ({Color background, Color foreground}) _stageColors(String status) {
    switch (status) {
      case 'completed':
        return (
          background: TailwindColors.tertiary.withValues(alpha: 0.20),
          foreground: TailwindColors.tertiaryFixed,
        );
      case 'running':
        return (
          background: TailwindColors.primary.withValues(alpha: 0.24),
          foreground: TailwindColors.primaryFixed,
        );
      case 'failed':
        return (
          background: TailwindColors.error.withValues(alpha: 0.20),
          foreground: TailwindColors.errorContainer,
        );
      default:
        return (
          background: TailwindColors.surfaceContainerHighest.withValues(
            alpha: 0.38,
          ),
          foreground: TailwindColors.inverseOnSurface,
        );
    }
  }

  IconData _stageIcon(String status) {
    switch (status) {
      case 'completed':
        return Icons.check_circle;
      case 'running':
        return Icons.autorenew;
      case 'failed':
        return Icons.error;
      default:
        return Icons.schedule;
    }
  }

  String _formatDuration(Duration? duration) {
    if (duration == null) {
      return 'Waiting for timer';
    }

    final hours = duration.inHours;
    final minutes = duration.inMinutes.remainder(60);
    final seconds = duration.inSeconds.remainder(60);

    if (hours > 0) {
      return '${hours}h ${minutes}m ${seconds}s';
    }
    if (minutes > 0) {
      return '${minutes}m ${seconds}s';
    }
    return '${seconds}s';
  }

  String _buildModelSummary(QueueItem item) {
    final labels = <String>[];
    if (item.usedLlm?.isNotEmpty == true) {
      labels.add('LLM ${item.usedLlm}');
    }
    if (item.usedVisionLlm?.isNotEmpty == true) {
      labels.add('Vision ${item.usedVisionLlm}');
    }
    return labels.join('  •  ');
  }

  Widget _buildRequestedStagePill(ProcessingStageProgress stage) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerHighest,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        stage.label,
        style: const TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w700,
          color: TailwindColors.onSurfaceVariant,
        ),
      ),
    );
  }

  Widget _buildQueueRow(QueueItem item) {
    final canOpenDetails = item.canViewResultDetails;
    final isRemoving = _removingItemId == item.id;
    final requestedStages = item.requestedStages;
    Color statusColor;
    Color statusBgColor;
    switch (item.status) {
      case 'completed':
        statusColor = TailwindColors.tertiary;
        statusBgColor = TailwindColors.tertiaryFixed;
        break;
      case 'partially_completed':
        statusColor = TailwindColors.secondary;
        statusBgColor = TailwindColors.secondaryContainer;
        break;
      case 'failed':
        statusColor = TailwindColors.error;
        statusBgColor = TailwindColors.errorContainer;
        break;
      case 'processing':
        statusColor = TailwindColors.primary;
        statusBgColor = TailwindColors.primaryContainer;
        break;
      default:
        statusColor = TailwindColors.outline;
        statusBgColor = TailwindColors.surfaceContainerHighest;
    }

    final addedStr = DateFormat(
      'MMM d, yyyy HH:mm',
    ).format(DateTime.fromMillisecondsSinceEpoch(item.requestedAtMs));

    final row = Container(
      decoration: const BoxDecoration(
        border: Border(top: BorderSide(color: TailwindColors.surfaceContainer)),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 16),
      child: Row(
        children: [
          Expanded(
            flex: 3,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  item.documentTitle,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: TailwindColors.onSurface,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  'Doc ID: ${item.documentId ?? "Unkn"}',
                  style: const TextStyle(
                    fontSize: 11,
                    fontFamily: 'monospace',
                    color: TailwindColors.onSurfaceVariant,
                  ),
                ),
                if (requestedStages.isNotEmpty) ...[
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 6,
                    runSpacing: 6,
                    children: requestedStages
                        .map(_buildRequestedStagePill)
                        .toList(),
                  ),
                ],
              ],
            ),
          ),
          Expanded(
            flex: 2,
            child: Align(
              alignment: Alignment.centerLeft,
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 10,
                  vertical: 4,
                ),
                decoration: BoxDecoration(
                  color: statusBgColor,
                  borderRadius: BorderRadius.circular(16),
                ),
                child: Text(
                  item.status.toUpperCase(),
                  style: TextStyle(
                    fontSize: 10,
                    fontWeight: FontWeight.bold,
                    color: statusColor,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
            ),
          ),
          Expanded(
            flex: 2,
            child: Text(
              addedStr,
              style: const TextStyle(
                fontSize: 12,
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ),
          Expanded(
            flex: 2,
            child: Text(
              '${item.attempts}',
              style: const TextStyle(
                fontSize: 12,
                fontFamily: 'monospace',
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ),
          SizedBox(
            width: 120,
            child: Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                if (item.status == 'pending')
                  Padding(
                    padding: const EdgeInsets.only(right: 12),
                    child: IconButton(
                      icon: const Icon(
                        Icons.play_arrow,
                        size: 24,
                        color: TailwindColors.primary,
                      ),
                      tooltip: 'Process now',
                      padding: EdgeInsets.zero,
                      constraints: const BoxConstraints(),
                      onPressed: isRemoving
                          ? null
                          : () => _processItem(item.id),
                    ),
                  )
                else if (item.status == 'failed' ||
                    item.status == 'partially_completed')
                  Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: OutlinedButton.icon(
                      onPressed: isRemoving
                          ? null
                          : () => _processItem(item.id),
                      icon: const Icon(
                        Icons.refresh,
                        size: 16,
                        color: TailwindColors.primary,
                      ),
                      label: const Text(
                        'Retry',
                        style: TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      style: OutlinedButton.styleFrom(
                        foregroundColor: TailwindColors.primary,
                        side: BorderSide(
                          color: TailwindColors.primary.withValues(alpha: 0.22),
                        ),
                        padding: const EdgeInsets.symmetric(
                          horizontal: 10,
                          vertical: 8,
                        ),
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(999),
                        ),
                        visualDensity: VisualDensity.compact,
                      ),
                    ),
                  ),
                if (item.canRemoveFromQueue)
                  IconButton(
                    onPressed: isRemoving ? null : () => _removeItem(item),
                    tooltip: 'Remove from queue',
                    padding: EdgeInsets.zero,
                    constraints: const BoxConstraints(),
                    icon: isRemoving
                        ? const SizedBox(
                            width: 20,
                            height: 20,
                            child: CircularProgressIndicator(strokeWidth: 2),
                          )
                        : const Icon(
                            Icons.delete_outline,
                            size: 22,
                            color: TailwindColors.error,
                          ),
                  ),
              ],
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
}
