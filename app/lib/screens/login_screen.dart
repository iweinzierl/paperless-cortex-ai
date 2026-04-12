import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import 'package:app/models/app_models.dart';
import 'package:app/providers/auth_provider.dart';
import 'package:app/services/api_service.dart';
import 'package:app/theme.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key});

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _formKey = GlobalKey<FormState>();
  final _usernameController = TextEditingController();
  final _passwordController = TextEditingController();

  bool _isLoading = false;
  bool _obscurePassword = true;
  String? _errorMessage;
  SystemStatusModel? _systemStatus;

  @override
  void dispose() {
    _usernameController.dispose();
    _passwordController.dispose();
    super.dispose();
  }

  @override
  void initState() {
    super.initState();
    _loadSystemStatus();
  }

  Future<void> _handleLogin() async {
    FocusScope.of(context).unfocus();
    final isValid = _formKey.currentState?.validate() ?? false;
    if (!isValid) {
      return;
    }

    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      await context.read<AuthProvider>().login(
        _usernameController.text.trim(),
        _passwordController.text,
      );
    } on ApiException catch (error) {
      if (mounted) {
        setState(() {
          _errorMessage = error.message;
        });
      }
    } catch (_) {
      if (mounted) {
        setState(() {
          _errorMessage = 'An unexpected error occurred during sign-in.';
        });
      }
    } finally {
      if (mounted) {
        setState(() {
          _isLoading = false;
        });
      }
    }
  }

  Future<void> _loadSystemStatus() async {
    try {
      final status = await context.read<ApiService>().getSystemStatus();
      if (!mounted) {
        return;
      }
      setState(() {
        _systemStatus = status;
      });
    } catch (_) {
      if (!mounted) {
        return;
      }
      setState(() {
        _systemStatus = null;
      });
    }
  }

  Future<void> _showForgotCredentialsDialog() {
    return showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Paperless credentials'),
        content: const Text(
          'Use the same Paperless-ngx username and password as in the web interface. If recovery is needed, do that in Paperless-ngx or the identity provider behind it.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Scaffold(
      body: Stack(
        children: [
          const _LoginBackground(),
          SafeArea(
            child: Center(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(
                  horizontal: 20,
                  vertical: 24,
                ),
                child: ConstrainedBox(
                  constraints: const BoxConstraints(maxWidth: 460),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      const _HeroHeader(),
                      const SizedBox(height: 20),
                      Card(
                        child: Padding(
                          padding: const EdgeInsets.fromLTRB(22, 22, 22, 20),
                          child: Form(
                            key: _formKey,
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.stretch,
                              children: [
                                Text(
                                  'Welcome back',
                                  style: theme.textTheme.titleLarge,
                                ),
                                const SizedBox(height: 6),
                                Text(
                                  'Authenticate with your Paperless-ngx credentials to access queue operations and document review on mobile.',
                                  style: theme.textTheme.bodyMedium,
                                ),
                                const SizedBox(height: 24),
                                if (_errorMessage != null) ...[
                                  Container(
                                    padding: const EdgeInsets.all(14),
                                    decoration: BoxDecoration(
                                      color: AppColors.errorSoft,
                                      borderRadius: BorderRadius.circular(18),
                                    ),
                                    child: Text(
                                      _errorMessage!,
                                      style: const TextStyle(
                                        color: AppColors.error,
                                        fontSize: 13,
                                        fontWeight: FontWeight.w700,
                                      ),
                                    ),
                                  ),
                                  const SizedBox(height: 18),
                                ],
                                Text(
                                  'USERNAME',
                                  style: theme.textTheme.labelMedium,
                                ),
                                const SizedBox(height: 8),
                                TextFormField(
                                  controller: _usernameController,
                                  keyboardType: TextInputType.emailAddress,
                                  textInputAction: TextInputAction.next,
                                  decoration: const InputDecoration(
                                    hintText: 'Enter your username',
                                    prefixIcon: Icon(Icons.person_outline),
                                  ),
                                  validator: (value) {
                                    final text = value?.trim() ?? '';
                                    if (text.isEmpty) {
                                      return 'Username is required.';
                                    }
                                    return null;
                                  },
                                ),
                                const SizedBox(height: 18),
                                Row(
                                  mainAxisAlignment:
                                      MainAxisAlignment.spaceBetween,
                                  children: [
                                    Text(
                                      'PASSWORD',
                                      style: theme.textTheme.labelMedium,
                                    ),
                                    TextButton(
                                      onPressed: _showForgotCredentialsDialog,
                                      child: const Text('Forgot credentials?'),
                                    ),
                                  ],
                                ),
                                const SizedBox(height: 8),
                                TextFormField(
                                  controller: _passwordController,
                                  obscureText: _obscurePassword,
                                  textInputAction: TextInputAction.done,
                                  onFieldSubmitted: (_) => _handleLogin(),
                                  decoration: InputDecoration(
                                    hintText: '••••••••',
                                    prefixIcon: const Icon(Icons.lock_outline),
                                    suffixIcon: IconButton(
                                      onPressed: () {
                                        setState(() {
                                          _obscurePassword = !_obscurePassword;
                                        });
                                      },
                                      icon: Icon(
                                        _obscurePassword
                                            ? Icons.visibility_outlined
                                            : Icons.visibility_off_outlined,
                                      ),
                                    ),
                                  ),
                                  validator: (value) {
                                    final text = value ?? '';
                                    if (text.isEmpty) {
                                      return 'Password is required.';
                                    }
                                    return null;
                                  },
                                ),
                                const SizedBox(height: 22),
                                ElevatedButton(
                                  onPressed: _isLoading ? null : _handleLogin,
                                  child: _isLoading
                                      ? const SizedBox(
                                          width: 22,
                                          height: 22,
                                          child: CircularProgressIndicator(
                                            strokeWidth: 2.4,
                                            valueColor:
                                                AlwaysStoppedAnimation<Color>(
                                                  AppColors.onPrimary,
                                                ),
                                          ),
                                        )
                                      : const Text('Authenticate Instance'),
                                ),
                                const SizedBox(height: 22),
                                _StatusSection(systemStatus: _systemStatus),
                              ],
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

class _HeroHeader extends StatelessWidget {
  const _HeroHeader();

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      children: [
        Container(
          width: 64,
          height: 64,
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(20),
            gradient: const LinearGradient(
              colors: [AppColors.primary, AppColors.primaryStrong],
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
            ),
            boxShadow: const [
              BoxShadow(
                color: Color(0x220051AE),
                blurRadius: 30,
                offset: Offset(0, 14),
              ),
            ],
          ),
          child: const Icon(
            Icons.hub_rounded,
            color: AppColors.onPrimary,
            size: 34,
          ),
        ),
        const SizedBox(height: 18),
        Text(
          'Cortex Graphite',
          style: theme.textTheme.displaySmall,
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 8),
        Text(
          'Technical Curator for Paperless-ngx',
          style: theme.textTheme.bodyLarge?.copyWith(color: AppColors.inkMuted),
          textAlign: TextAlign.center,
        ),
        const SizedBox(height: 12),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.72),
            borderRadius: BorderRadius.circular(999),
            border: Border.all(color: AppColors.outline.withValues(alpha: 0.5)),
          ),
          child: const Text(
            'Mobile control surface for queue triage and AI-assisted review',
            textAlign: TextAlign.center,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w700,
              color: AppColors.inkMuted,
            ),
          ),
        ),
      ],
    );
  }
}

class _StatusSection extends StatelessWidget {
  const _StatusSection({required this.systemStatus});

  final SystemStatusModel? systemStatus;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            Expanded(
              child: Divider(color: AppColors.outline.withValues(alpha: 0.6)),
            ),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 10),
              child: Text(
                'INSTANCE STATUS',
                style: theme.textTheme.labelMedium,
              ),
            ),
            Expanded(
              child: Divider(color: AppColors.outline.withValues(alpha: 0.6)),
            ),
          ],
        ),
        const SizedBox(height: 16),
        _StatusTile(
          title: 'Backend',
          subtitle: _statusLabel('Backend', systemStatus?.backend),
          color: _statusColor(systemStatus?.backend),
          icon: Icons.cloud_done_outlined,
        ),
        const SizedBox(height: 10),
        _StatusTile(
          title: 'Paperless',
          subtitle: _statusLabel('Paperless', systemStatus?.paperless),
          color: _statusColor(systemStatus?.paperless),
          icon: Icons.inventory_2_outlined,
        ),
        const SizedBox(height: 10),
        _StatusTile(
          title: 'Ollama',
          subtitle: _statusLabel('Ollama', systemStatus?.ollama),
          color: _statusColor(systemStatus?.ollama),
          icon: Icons.memory_outlined,
        ),
      ],
    );
  }
}

Color _statusColor(DependencyStatusModel? status) {
  if (status == null) {
    return AppColors.secondary;
  }
  if (!status.configured) {
    return AppColors.secondary;
  }
  return status.healthy ? AppColors.tertiary : AppColors.error;
}

String _statusLabel(String name, DependencyStatusModel? status) {
  if (status == null) {
    return '$name status is being checked.';
  }
  if (!status.configured) {
    return '$name is not configured.';
  }
  if (status.message.trim().isNotEmpty) {
    return status.message;
  }
  return status.healthy ? '$name is connected.' : '$name is unavailable.';
}

class _StatusTile extends StatelessWidget {
  const _StatusTile({
    required this.title,
    required this.subtitle,
    required this.color,
    required this.icon,
  });

  final String title;
  final String subtitle;
  final Color color;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
      decoration: BoxDecoration(
        color: AppColors.surfaceMuted,
        borderRadius: BorderRadius.circular(18),
      ),
      child: Row(
        children: [
          Container(
            width: 42,
            height: 42,
            decoration: BoxDecoration(
              color: color.withValues(alpha: 0.12),
              borderRadius: BorderRadius.circular(14),
            ),
            child: Icon(icon, color: color),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w700,
                    color: AppColors.ink,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  subtitle,
                  style: const TextStyle(
                    fontSize: 12,
                    height: 1.35,
                    color: AppColors.inkMuted,
                  ),
                ),
              ],
            ),
          ),
          Container(
            width: 10,
            height: 10,
            decoration: BoxDecoration(color: color, shape: BoxShape.circle),
          ),
        ],
      ),
    );
  }
}

class _LoginBackground extends StatelessWidget {
  const _LoginBackground();

  @override
  Widget build(BuildContext context) {
    return DecoratedBox(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          colors: [Color(0xFFF6F8FC), Color(0xFFEAF0FA)],
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
        ),
      ),
      child: Stack(
        fit: StackFit.expand,
        children: [
          Positioned(
            top: -80,
            left: -70,
            child: _GlowOrb(
              size: 220,
              color: AppColors.primary.withValues(alpha: 0.12),
            ),
          ),
          Positioned(
            top: 140,
            right: -90,
            child: _GlowOrb(
              size: 240,
              color: AppColors.tertiary.withValues(alpha: 0.1),
            ),
          ),
          Positioned(
            bottom: -110,
            left: 40,
            child: _GlowOrb(
              size: 280,
              color: AppColors.primaryStrong.withValues(alpha: 0.08),
            ),
          ),
        ],
      ),
    );
  }
}

class _GlowOrb extends StatelessWidget {
  const _GlowOrb({required this.size, required this.color});

  final double size;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        gradient: RadialGradient(colors: [color, color.withValues(alpha: 0)]),
      ),
    );
  }
}
