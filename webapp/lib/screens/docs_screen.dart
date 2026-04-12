import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:webapp/theme.dart';

class DocsScreen extends StatelessWidget {
  const DocsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.symmetric(horizontal: 40, vertical: 32),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            'Operator Notes',
            style: TextStyle(
              fontSize: 30,
              fontWeight: FontWeight.w800,
              color: TailwindColors.onSurface,
              letterSpacing: -0.5,
            ),
          ),
          const SizedBox(height: 8),
          const SizedBox(
            width: 760,
            child: Text(
              'This interface configures the backend queue processor. Documents are queued from Paperless, processed in stages, and the resulting suggestions remain available for review before any future writeback flow.',
              style: TextStyle(
                fontSize: 14,
                color: TailwindColors.onSurfaceVariant,
              ),
            ),
          ),
          const SizedBox(height: 32),
          const _DocsCard(
            title: 'Processing Flow',
            bullets: [
              'Documents enter the queue from the Paperless webhook.',
              'Text extraction runs first, followed by the requested LLM suggestion stages.',
              'Results are stored on queue items so they can be reviewed in the dashboard and queue screens.',
            ],
          ),
          const SizedBox(height: 24),
          const _DocsCard(
            title: 'Required Configuration',
            bullets: [
              'Set the Paperless URL and token before using queue and metadata features.',
              'Set the Ollama URL plus default and vision models for extraction and suggestions.',
              'Choose trigger and process tags so the backend knows which stages should run.',
            ],
          ),
          const SizedBox(height: 24),
          const _DocsCard(
            title: 'Queue Behavior',
            bullets: [
              'Manual mode requires users to trigger processing from the queue screen.',
              'Auto mode polls for idle work on the configured interval, then drains queued items continuously until the queue is empty.',
              'Failed and partially completed items can be retried after review.',
            ],
          ),
          const SizedBox(height: 24),
          Row(
            children: [
              ElevatedButton(
                onPressed: () => context.go('/configuration'),
                style: ElevatedButton.styleFrom(
                  backgroundColor: TailwindColors.primary,
                  foregroundColor: TailwindColors.onPrimary,
                  padding: const EdgeInsets.symmetric(
                    horizontal: 24,
                    vertical: 16,
                  ),
                ),
                child: const Text('Open Configuration'),
              ),
              const SizedBox(width: 16),
              OutlinedButton(
                onPressed: () => context.go('/queue'),
                style: OutlinedButton.styleFrom(
                  foregroundColor: TailwindColors.onSurfaceVariant,
                  padding: const EdgeInsets.symmetric(
                    horizontal: 24,
                    vertical: 16,
                  ),
                ),
                child: const Text('Open Queue'),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

class _DocsCard extends StatelessWidget {
  final String title;
  final List<String> bullets;

  const _DocsCard({required this.title, required this.bullets});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(24),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerLowest,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(
          color: TailwindColors.outlineVariant.withValues(alpha: 0.15),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            title,
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w700,
              color: TailwindColors.onSurface,
            ),
          ),
          const SizedBox(height: 16),
          ...bullets.map(
            (bullet) => Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Padding(
                    padding: EdgeInsets.only(top: 6),
                    child: Icon(
                      Icons.circle,
                      size: 6,
                      color: TailwindColors.primary,
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(
                      bullet,
                      style: const TextStyle(
                        fontSize: 14,
                        height: 1.5,
                        color: TailwindColors.onSurfaceVariant,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }
}
