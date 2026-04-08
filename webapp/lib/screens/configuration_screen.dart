import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:webapp/theme.dart';
import 'package:webapp/services/api_service.dart';
import 'package:webapp/models/models.dart';

class ConfigurationScreen extends StatefulWidget {
  const ConfigurationScreen({super.key});

  @override
  State<ConfigurationScreen> createState() => _ConfigurationScreenState();
}

class _ConfigurationScreenState extends State<ConfigurationScreen> {
  bool _isLoading = true;
  String? _error;
  String? _successMsg;

  // Controllers
  final _intervalController = TextEditingController();
  final _apiUrlController = TextEditingController();
  final _apiTokenController = TextEditingController();

  final _triggerController = TextEditingController();
  final _ocrController = TextEditingController();
  final _visionController = TextEditingController();
  final _corrController = TextEditingController();
  final _typeController = TextEditingController();
  final _tagsController = TextEditingController();
  final _compController = TextEditingController();

  final _ollamaUrlController = TextEditingController();

  String _processingMode = 'manual';
  String _defaultLlm = 'mistral:latest';
  String _visionLlm = 'llava:latest';

  @override
  void initState() {
    super.initState();
    _loadData();
  }

  Future<void> _loadData() async {
    setState(() {
      _isLoading = true;
      _error = null;
      _successMsg = null;
    });

    try {
      final api = context.read<ApiService>();
      final config = await api.getConfig();
      if (mounted) {
        setState(() {
          _processingMode = config.engine.processingMode;
          _intervalController.text = config.engine.processingIntervalSeconds
              .toString();

          _apiUrlController.text = config.paperless.paperlessUrl;
          _apiTokenController.text = config.paperless.paperlessToken;

          _triggerController.text = config.process.processTriggerTag;
          _ocrController.text = config.process.forceOcrTag;
          _visionController.text = config.process.forceVisionTag;
          _corrController.text = config.process.processCorrespondentTag;
          _typeController.text = config.process.processDocumentTypeTag;
          _tagsController.text = config.process.processDocumentTagsTag;
          _compController.text = config.process.processCompletedTag;

          _ollamaUrlController.text = config.llms.ollamaUrl;
          if (config.llms.defaultLlm.isNotEmpty)
            _defaultLlm = config.llms.defaultLlm;
          if (config.llms.visionLlm.isNotEmpty)
            _visionLlm = config.llms.visionLlm;

          _isLoading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Failed to load config: \$e';
          _isLoading = false;
        });
      }
    }
  }

  Future<void> _saveData() async {
    setState(() {
      _isLoading = true;
      _error = null;
      _successMsg = null;
    });

    try {
      final api = context.read<ApiService>();
      final config = BackendConfig(
        engine: EngineConfig(
          processingMode: _processingMode,
          processingIntervalSeconds:
              int.tryParse(_intervalController.text) ?? 30,
        ),
        process: ProcessConfig(
          processTriggerTag: _triggerController.text,
          forceOcrTag: _ocrController.text,
          forceVisionTag: _visionController.text,
          processCorrespondentTag: _corrController.text,
          processDocumentTypeTag: _typeController.text,
          processDocumentTagsTag: _tagsController.text,
          processCompletedTag: _compController.text,
        ),
        paperless: PaperlessConfig(
          paperlessUrl: _apiUrlController.text,
          paperlessToken: _apiTokenController.text,
        ),
        llms: LLMConfig(
          ollamaUrl: _ollamaUrlController.text,
          defaultLlm: _defaultLlm,
          visionLlm: _visionLlm,
        ),
      );

      await api.putConfig(config);
      if (mounted) {
        setState(() {
          _successMsg = 'Configuration saved successfully.';
          _isLoading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _error = 'Failed to save config: \$e';
          _isLoading = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_isLoading) {
      return const Center(child: CircularProgressIndicator());
    }

    return SingleChildScrollView(
      padding: const EdgeInsets.symmetric(horizontal: 40.0, vertical: 32.0),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Editorial Header
          const Text(
            'System Configuration',
            style: TextStyle(
              fontSize: 30,
              fontWeight: FontWeight.w800,
              letterSpacing: -0.5,
              color: TailwindColors.onSurface,
            ),
          ),
          const SizedBox(height: 8),
          const SizedBox(
            width: 650,
            child: Text(
              'Refine the operational parameters of the Cortex Graphite engine. These settings govern the orchestration between Paperless-ngx and local LLM instances.',
              style: TextStyle(
                color: TailwindColors.onSurfaceVariant,
                fontSize: 14,
              ),
            ),
          ),
          const SizedBox(height: 24),

          if (_error != null)
            Container(
              padding: const EdgeInsets.all(16),
              color: TailwindColors.errorContainer,
              child: Text(
                _error!,
                style: const TextStyle(color: TailwindColors.error),
              ),
            ),
          if (_successMsg != null)
            Container(
              padding: const EdgeInsets.all(16),
              color: TailwindColors.tertiaryFixedDim,
              child: Text(
                _successMsg!,
                style: const TextStyle(
                  color: TailwindColors.onTertiaryFixedVariant,
                ),
              ),
            ),

          const SizedBox(height: 24),

          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Left Column (Section 1 and 3)
              Expanded(
                flex: 4,
                child: Column(
                  children: [
                    _buildSectionBlock(
                      title: 'Paperless-ngx',
                      icon: Icons.description,
                      iconBg: TailwindColors.tertiaryFixed,
                      iconColor: TailwindColors.tertiary,
                      children: [
                        _buildInputField('API URL', null, _apiUrlController),
                        const SizedBox(height: 24),
                        _buildInputField(
                          'API TOKEN',
                          null,
                          _apiTokenController,
                          isPassword: true,
                        ),
                      ],
                    ),
                    const SizedBox(height: 32),
                    _buildSectionBlock(
                      title: 'Engine',
                      icon: Icons.settings_input_component,
                      iconBg: TailwindColors.primaryFixed,
                      iconColor: TailwindColors.primary,
                      children: [
                        _buildDropdownField(
                          'PROCESSING MODE',
                          'Determines if documents trigger analysis immediately.',
                          ['manual', 'auto'],
                          _processingMode,
                          (v) => _processingMode = v!,
                        ),
                        const SizedBox(height: 24),
                        _buildInputField(
                          'INTERVAL (SECONDS)',
                          'Frequency of queue synchronization.',
                          _intervalController,
                          isNumber: true,
                        ),
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 32),

              // Right Column (Section 2 and 4)
              Expanded(
                flex: 8,
                child: Column(
                  children: [
                    _buildSectionBlock(
                      title: 'Process Taxonomy',
                      icon: Icons.label,
                      iconBg: TailwindColors.secondaryContainer,
                      iconColor: TailwindColors.secondary,
                      badge: 'METADATA MAPPING',
                      children: [
                        Row(
                          children: [
                            Expanded(
                              child: _buildInputField(
                                'TRIGGER',
                                null,
                                _triggerController,
                              ),
                            ),
                            const SizedBox(width: 32),
                            Expanded(
                              child: _buildInputField(
                                'OCR',
                                null,
                                _ocrController,
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 24),
                        Row(
                          children: [
                            Expanded(
                              child: _buildInputField(
                                'VISION',
                                null,
                                _visionController,
                              ),
                            ),
                            const SizedBox(width: 32),
                            Expanded(
                              child: _buildInputField(
                                'CORRESPONDENT',
                                null,
                                _corrController,
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 24),
                        Row(
                          children: [
                            Expanded(
                              child: _buildInputField(
                                'TYPE',
                                null,
                                _typeController,
                              ),
                            ),
                            const SizedBox(width: 32),
                            Expanded(
                              child: _buildInputField(
                                'TAGS',
                                null,
                                _tagsController,
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 24),
                        _buildInputField('COMPLETED', null, _compController),
                      ],
                    ),
                    const SizedBox(height: 32),
                    _buildSectionBlock(
                      title: 'Intelligence Sources (Ollama)',
                      icon: Icons.psychology,
                      iconBg: TailwindColors.primaryFixedDim,
                      iconColor: TailwindColors.primary,
                      children: [
                        Container(
                          padding: const EdgeInsets.all(24),
                          decoration: BoxDecoration(
                            color: TailwindColors.surfaceContainerLow,
                            borderRadius: BorderRadius.circular(12),
                            border: const Border(
                              left: BorderSide(
                                color: TailwindColors.surfaceTint,
                                width: 4,
                              ),
                            ),
                          ),
                          child: Row(
                            children: [
                              Expanded(
                                child: Column(
                                  children: [
                                    _buildInnerField(
                                      'ENDPOINT URL',
                                      _ollamaUrlController,
                                    ),
                                    const SizedBox(height: 24),
                                    _buildInnerDropdown(
                                      'DEFAULT LLM',
                                      [
                                        'llama3:8b-instruct',
                                        'mistral:latest',
                                        'gemma:7b',
                                      ],
                                      _defaultLlm,
                                      (v) => _defaultLlm = v!,
                                    ),
                                  ],
                                ),
                              ),
                              const SizedBox(width: 32),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  mainAxisAlignment: MainAxisAlignment.end,
                                  children: [
                                    _buildInnerDropdown(
                                      'VISION LLM',
                                      ['llava:latest', 'bakllava:latest'],
                                      _visionLlm,
                                      (v) => _visionLlm = v!,
                                    ),
                                    const SizedBox(height: 24),
                                    Row(
                                      children: [
                                        Container(
                                          width: 8,
                                          height: 8,
                                          decoration: const BoxDecoration(
                                            color: TailwindColors.tertiary,
                                            shape: BoxShape.circle,
                                          ),
                                        ),
                                        const SizedBox(width: 8),
                                        const Text(
                                          'CONNECTION STABLE',
                                          style: TextStyle(
                                            fontSize: 11,
                                            fontWeight: FontWeight.bold,
                                            color: TailwindColors.tertiary,
                                            letterSpacing: 0.5,
                                          ),
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
                    const SizedBox(height: 48),

                    // Actions Footer
                    Row(
                      mainAxisAlignment: MainAxisAlignment.end,
                      children: [
                        TextButton(
                          onPressed: _loadData,
                          style: TextButton.styleFrom(
                            foregroundColor: TailwindColors.onSurfaceVariant,
                            padding: const EdgeInsets.symmetric(
                              horizontal: 32,
                              vertical: 16,
                            ),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(12),
                            ),
                          ),
                          child: const Text(
                            'Revert Changes',
                            style: TextStyle(fontWeight: FontWeight.bold),
                          ),
                        ),
                        const SizedBox(width: 16),
                        ElevatedButton(
                          onPressed: _saveData,
                          style: ElevatedButton.styleFrom(
                            backgroundColor: TailwindColors.primary,
                            foregroundColor: TailwindColors.onPrimary,
                            padding: const EdgeInsets.symmetric(
                              horizontal: 40,
                              vertical: 16,
                            ),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(12),
                            ),
                            elevation: 4,
                            shadowColor: TailwindColors.primaryFixedDim,
                          ),
                          child: const Text(
                            'Commit Configuration',
                            style: TextStyle(fontWeight: FontWeight.bold),
                          ),
                        ),
                      ],
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

  Widget _buildSectionBlock({
    required String title,
    required IconData icon,
    required Color iconBg,
    required Color iconColor,
    String? badge,
    required List<Widget> children,
  }) {
    return Container(
      padding: const EdgeInsets.all(32),
      decoration: BoxDecoration(
        color: TailwindColors.surfaceContainerLowest,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: TailwindColors.outlineVariant.withValues(alpha: 0.15),
        ),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.02),
            blurRadius: 4,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Row(
                children: [
                  Container(
                    padding: const EdgeInsets.all(8),
                    decoration: BoxDecoration(
                      color: iconBg,
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: Icon(icon, color: iconColor, size: 20),
                  ),
                  const SizedBox(width: 12),
                  Text(
                    title,
                    style: const TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.bold,
                      color: TailwindColors.onSurface,
                    ),
                  ),
                ],
              ),
              if (badge != null)
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 12,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: TailwindColors.primaryFixed,
                    borderRadius: BorderRadius.circular(16),
                  ),
                  child: Text(
                    badge,
                    style: const TextStyle(
                      fontSize: 10,
                      fontWeight: FontWeight.bold,
                      color: TailwindColors.onPrimaryFixedVariant,
                    ),
                  ),
                ),
            ],
          ),
          const SizedBox(height: 32),
          ...children,
        ],
      ),
    );
  }

  Widget _buildInputField(
    String label,
    String? hint,
    TextEditingController controller, {
    bool isNumber = false,
    bool isPassword = false,
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 12,
            fontWeight: FontWeight.bold,
            color: TailwindColors.onSurfaceVariant,
            letterSpacing: 0.5,
          ),
        ),
        const SizedBox(height: 8),
        TextField(
          controller: controller,
          obscureText: isPassword,
          keyboardType: isNumber ? TextInputType.number : TextInputType.text,
          style: const TextStyle(fontSize: 14, fontFamily: 'monospace'),
          decoration: InputDecoration(
            filled: true,
            fillColor: TailwindColors.surfaceContainerHighest,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide.none,
            ),
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 16,
            ),
            suffixIcon: isPassword
                ? const Icon(
                    Icons.visibility,
                    color: TailwindColors.outline,
                    size: 20,
                  )
                : null,
          ),
        ),
        if (hint != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              hint,
              style: const TextStyle(
                fontSize: 11,
                color: TailwindColors.outline,
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildDropdownField(
    String label,
    String? hint,
    List<String> options,
    String selectedValue,
    ValueChanged<String?> onChanged,
  ) {
    if (!options.contains(selectedValue)) {
      options = [...options, selectedValue];
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 12,
            fontWeight: FontWeight.bold,
            color: TailwindColors.onSurfaceVariant,
            letterSpacing: 0.5,
          ),
        ),
        const SizedBox(height: 8),
        DropdownButtonFormField<String>(
          initialValue: selectedValue,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w500,
            color: TailwindColors.onSurface,
          ),
          decoration: InputDecoration(
            filled: true,
            fillColor: TailwindColors.surfaceContainerHighest,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide.none,
            ),
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 16,
            ),
          ),
          items: options
              .map((opt) => DropdownMenuItem(value: opt, child: Text(opt)))
              .toList(),
          onChanged: onChanged,
        ),
        if (hint != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              hint,
              style: const TextStyle(
                fontSize: 11,
                color: TailwindColors.outline,
              ),
            ),
          ),
      ],
    );
  }

  Widget _buildInnerField(String label, TextEditingController controller) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w900,
            color: TailwindColors.primary,
            letterSpacing: 1.0,
          ),
        ),
        const SizedBox(height: 8),
        TextField(
          controller: controller,
          style: const TextStyle(fontSize: 14, fontFamily: 'monospace'),
          decoration: InputDecoration(
            filled: true,
            fillColor: TailwindColors.surfaceContainerLowest,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: BorderSide.none,
            ),
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 12,
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildInnerDropdown(
    String label,
    List<String> options,
    String selectedValue,
    ValueChanged<String?> onChanged,
  ) {
    if (!options.contains(selectedValue)) {
      options = [...options, selectedValue];
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w900,
            color: TailwindColors.primary,
            letterSpacing: 1.0,
          ),
        ),
        const SizedBox(height: 8),
        DropdownButtonFormField<String>(
          initialValue: selectedValue,
          style: const TextStyle(
            fontSize: 14,
            fontFamily: 'monospace',
            color: TailwindColors.onSurface,
          ),
          decoration: InputDecoration(
            filled: true,
            fillColor: TailwindColors.surfaceContainerLowest,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: BorderSide.none,
            ),
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 16,
              vertical: 12,
            ),
          ),
          items: options
              .map((opt) => DropdownMenuItem(value: opt, child: Text(opt)))
              .toList(),
          onChanged: onChanged,
        ),
      ],
    );
  }
}
