import 'package:flutter/material.dart';

import 'package:app/models/app_models.dart';
import 'package:app/services/api_service.dart';
import 'package:app/theme.dart';

class QueueScreen extends StatefulWidget {
  const QueueScreen({super.key, required this.apiService});

  final ApiService apiService;

  @override
  State<QueueScreen> createState() => _QueueScreenState();
}

class _QueueScreenState extends State<QueueScreen> {
  late Future<List<QueueItem>> _future;
  String _statusFilter = '';

  static const _filters = <String, String>{
    '': 'All',
    'pending': 'Pending',
    'processing': 'Processing',
    'completed': 'Completed',
    'partially_completed': 'Partial',
    'failed': 'Failed',
  };

  @override
  void initState() {
    super.initState();
    _future = _load();
  }

  Future<List<QueueItem>> _load() {
    return widget.apiService.getQueue(status: _statusFilter, limit: 30);
  }

  Future<void> _refresh() async {
    final future = _load();
    setState(() {
      _future = future;
    });
    await future;
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return RefreshIndicator(
      onRefresh: _refresh,
      child: FutureBuilder<List<QueueItem>>(
        future: _future,
        builder: (context, snapshot) {
          if (snapshot.connectionState != ConnectionState.done) {
            return const Center(child: CircularProgressIndicator());
          }

          if (snapshot.hasError) {
            return ListView(
              physics: const AlwaysScrollableScrollPhysics(),
              padding: const EdgeInsets.all(20),
              children: [
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(20),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Queue unavailable',
                          style: theme.textTheme.titleLarge,
                        ),
                        const SizedBox(height: 8),
                        Text(
                          snapshot.error.toString(),
                          style: theme.textTheme.bodyMedium,
                        ),
                        const SizedBox(height: 18),
                        ElevatedButton(
                          onPressed: _refresh,
                          child: const Text('Retry'),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            );
          }

          final items = snapshot.data!;
          return ListView(
            physics: const AlwaysScrollableScrollPhysics(),
            padding: const EdgeInsets.fromLTRB(20, 20, 20, 28),
            children: [
              Text('Queue', style: theme.textTheme.headlineSmall),
              const SizedBox(height: 6),
              Text(
                'Recent processing work optimized for quick inspection on a phone.',
                style: theme.textTheme.bodyMedium,
              ),
              const SizedBox(height: 18),
              SizedBox(
                height: 40,
                child: ListView.separated(
                  scrollDirection: Axis.horizontal,
                  itemCount: _filters.length,
                  separatorBuilder: (_, _) => const SizedBox(width: 8),
                  itemBuilder: (context, index) {
                    final entry = _filters.entries.elementAt(index);
                    final selected = entry.key == _statusFilter;
                    return FilterChip(
                      label: Text(entry.value),
                      selected: selected,
                      onSelected: (_) {
                        setState(() {
                          _statusFilter = entry.key;
                          _future = _load();
                        });
                      },
                    );
                  },
                ),
              ),
              const SizedBox(height: 18),
              if (items.isEmpty)
                const Card(
                  child: Padding(
                    padding: EdgeInsets.all(20),
                    child: Text(
                      'No queue items match the current filter.',
                      style: TextStyle(color: AppColors.inkMuted),
                    ),
                  ),
                )
              else
                ...items.map(
                  (item) => Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: _QueueTile(item: item),
                  ),
                ),
            ],
          );
        },
      ),
    );
  }
}

class _QueueTile extends StatelessWidget {
  const _QueueTile({required this.item});

  final QueueItem item;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final statusColor = switch (item.status) {
      'completed' => AppColors.tertiary,
      'partially_completed' => const Color(0xFF9A6700),
      'failed' => AppColors.error,
      'processing' => AppColors.primary,
      _ => AppColors.secondary,
    };

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(18),
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
                        style: theme.textTheme.bodyLarge,
                      ),
                      const SizedBox(height: 4),
                      Text(
                        'Source: ${item.source.isEmpty ? 'unknown' : item.source}',
                        style: theme.textTheme.bodyMedium,
                      ),
                    ],
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 6,
                  ),
                  decoration: BoxDecoration(
                    color: statusColor.withValues(alpha: 0.12),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(
                    item.status.replaceAll('_', ' '),
                    style: TextStyle(
                      color: statusColor,
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 10),
            Text(
              item.resultSummary?.trim().isNotEmpty == true
                  ? item.resultSummary!
                  : item.lastError?.trim().isNotEmpty == true
                  ? item.lastError!
                  : 'Queue item #${item.id} is ready for deeper mobile detail views.',
              style: theme.textTheme.bodyMedium,
            ),
            if (item.applyStatus.trim().isNotEmpty) ...[
              const SizedBox(height: 10),
              Text(
                'Apply status: ${item.applyStatus.replaceAll('_', ' ')}',
                style: theme.textTheme.labelMedium?.copyWith(
                  color: AppColors.inkMuted,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
