import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:webapp/theme.dart';
import 'package:webapp/services/api_service.dart';
import 'package:webapp/models/models.dart';
import 'package:intl/intl.dart';

class QueueScreen extends StatefulWidget {
  const QueueScreen({super.key});

  @override
  State<QueueScreen> createState() => _QueueScreenState();
}

class _QueueScreenState extends State<QueueScreen> {
  bool _isLoading = true;
  String? _error;
  List<QueueItem> _items = [];
  String _filterStatus = 'all';

  @override
  void initState() {
    super.initState();
    _loadData();
  }

  Future<void> _loadData() async {
    setState(() {
      _isLoading = true;
      _error = null;
    });

    try {
      final api = context.read<ApiService>();
      final statusQuery = _filterStatus == 'all' ? null : _filterStatus;
      final items = await api.getQueue(status: statusQuery, limit: 100);
      if (mounted) {
        setState(() {
          _items = items;
          _isLoading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Failed to load queue: \$e';
          _isLoading = false;
        });
      }
    }
  }

  Future<void> _processItem(int id) async {
    try {
      final api = context.read<ApiService>();
      await api.processQueueItem(id);
      _loadData(); // Refresh immediately after enqueing processing
    } catch (e) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text('Processing failed: \$e')));
    }
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
    int errorCount = _items.where((i) => i.status == 'failed').length;

    return SingleChildScrollView(
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
                    '\${_items.length} items currently in log. Pending processes execute asynchronously.',
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
                color: TailwindColors.outlineVariant.withValues(alpha: 0.15),
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
                        child: const TextField(
                          style: TextStyle(fontSize: 12),
                          decoration: InputDecoration(
                            hintText: 'Search documents...',
                            prefixIcon: Icon(
                              Icons.search,
                              size: 16,
                              color: TailwindColors.outline,
                            ),
                            border: InputBorder.none,
                            contentPadding: EdgeInsets.symmetric(vertical: 12),
                          ),
                        ),
                      ),
                      const SizedBox(width: 8),
                      OutlinedButton.icon(
                        onPressed: _loadData,
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
                Container(color: TailwindColors.surfaceContainer, height: 1),

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
                      const SizedBox(width: 40), // actions spacer
                    ],
                  ),
                ),

                // Rows
                if (_items.isEmpty)
                  const Padding(
                    padding: EdgeInsets.all(40),
                    child: Center(
                      child: Text(
                        'Queue is empty matching these filters.',
                        style: TextStyle(
                          fontSize: 14,
                          color: TailwindColors.onSurfaceVariant,
                        ),
                      ),
                    ),
                  ),

                ..._items.map((item) => _buildQueueRow(item)),

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
                      const Text(
                        'Showing most recent 100 queue entries',
                        style: TextStyle(
                          fontSize: 12,
                          color: TailwindColors.outline,
                        ),
                      ),
                      Row(
                        children: [
                          IconButton(
                            onPressed: () {},
                            icon: const Icon(Icons.chevron_left, size: 20),
                            color: TailwindColors.onSurfaceVariant,
                            constraints: const BoxConstraints(),
                          ),
                          const SizedBox(width: 8),
                          const Text(
                            'Page 1 of 1',
                            style: TextStyle(
                              fontSize: 12,
                              fontWeight: FontWeight.w600,
                              color: TailwindColors.onSurface,
                            ),
                          ),
                          const SizedBox(width: 8),
                          IconButton(
                            onPressed: () {},
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
        ],
      ),
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

  Widget _buildQueueRow(QueueItem item) {
    Color statusColor;
    Color statusBgColor;
    switch (item.status) {
      case 'completed':
        statusColor = TailwindColors.tertiary;
        statusBgColor = TailwindColors.tertiaryFixed;
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

    return Container(
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
                  'Doc ID: \${item.documentId ?? "Unkn"}',
                  style: const TextStyle(
                    fontSize: 11,
                    fontFamily: 'monospace',
                    color: TailwindColors.onSurfaceVariant,
                  ),
                ),
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
              '\${item.attempts}',
              style: const TextStyle(
                fontSize: 12,
                fontFamily: 'monospace',
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ),
          SizedBox(
            width: 40,
            child: item.status == 'pending' || item.status == 'failed'
                ? IconButton(
                    icon: const Icon(
                      Icons.play_arrow,
                      size: 20,
                      color: TailwindColors.primary,
                    ),
                    tooltip: 'Process Now',
                    padding: EdgeInsets.zero,
                    constraints: const BoxConstraints(),
                    onPressed: () => _processItem(item.id),
                  )
                : null,
          ),
        ],
      ),
    );
  }
}
