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
  final _createdDateController = TextEditingController();
  final _corrController = TextEditingController();
  final _typeController = TextEditingController();
  final _tagsController = TextEditingController();
  final _titleController = TextEditingController();
  final _compController = TextEditingController();

  final _ollamaUrlController = TextEditingController();

  String _processingMode = 'manual';
  String _defaultLlm = '';
  String _visionLlm = '';
  List<String> _availableModels = [];
  List<DocumentTag> _availableTags = [];
  bool _ollamaHealthy = false;

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

      bool isHealthy = false;
      List<String> availableMods = [];
      try {
        final modelsResp = await api.getModels();
        availableMods = modelsResp.models.map((m) => m.name).toList();
        isHealthy = true;
      } catch (e) {
        print("Failed to fetch models: $e");
      }

      List<DocumentTag> availableTags = [];
      try {
        availableTags = await api.getPaperlessTags();
      } catch (e) {
        print("Failed to fetch paperless tags: $e");
      }

      if (mounted) {
        setState(() {
          _ollamaHealthy = isHealthy;
          _availableModels = availableMods;
          _availableTags = availableTags;
          _processingMode = config.engine.processingMode;
          _intervalController.text = config.engine.processingIntervalSeconds
              .toString();

          _apiUrlController.text = config.paperless.paperlessUrl;
          _apiTokenController.text = config.paperless.paperlessToken;

          _triggerController.text = config.process.processTriggerTag;
          _ocrController.text = config.process.forceOcrTag;
          _visionController.text = config.process.forceVisionTag;
          _createdDateController.text = config.process.processCreatedDateTag;
          _corrController.text = config.process.processCorrespondentTag;
          _typeController.text = config.process.processDocumentTypeTag;
          _tagsController.text = config.process.processDocumentTagsTag;
          _titleController.text = config.process.processTitleTag;
          _compController.text = config.process.processCompletedTag;

          _ollamaUrlController.text = config.llms.ollamaUrl;
          _defaultLlm = config.llms.defaultLlm.isNotEmpty
              ? config.llms.defaultLlm
              : (availableMods.isNotEmpty ? availableMods.first : '');
          _visionLlm = config.llms.visionLlm.isNotEmpty
              ? config.llms.visionLlm
              : (availableMods.isNotEmpty ? availableMods.first : '');

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
          processCreatedDateTag: _createdDateController.text,
          processCorrespondentTag: _corrController.text,
          processDocumentTypeTag: _typeController.text,
          processDocumentTagsTag: _tagsController.text,
          processTitleTag: _titleController.text,
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
                              child: _buildTagAutocompleteField(
                                'TRIGGER',
                                null,
                                _triggerController,
                              ),
                            ),
                            const SizedBox(width: 32),
                            Expanded(
                              child: _buildTagAutocompleteField(
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
                              child: _buildTagAutocompleteField(
                                'VISION',
                                null,
                                _visionController,
                              ),
                            ),
                            const SizedBox(width: 32),
                            Expanded(
                              child: _buildTagAutocompleteField(
                                'CREATED DATE',
                                null,
                                _createdDateController,
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 24),
                        Row(
                          children: [
                            Expanded(
                              child: _buildTagAutocompleteField(
                                'TITLE',
                                null,
                                _titleController,
                              ),
                            ),
                            const SizedBox(width: 32),
                            Expanded(
                              child: _buildTagAutocompleteField(
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
                              child: _buildTagAutocompleteField(
                                'TYPE',
                                null,
                                _typeController,
                              ),
                            ),
                            const SizedBox(width: 32),
                            Expanded(
                              child: _buildTagAutocompleteField(
                                'TAGS',
                                null,
                                _tagsController,
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 24),
                        _buildTagAutocompleteField(
                          'COMPLETED',
                          null,
                          _compController,
                        ),
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
                                      _availableModels,
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
                                      _availableModels,
                                      _visionLlm,
                                      (v) => _visionLlm = v!,
                                    ),
                                    const SizedBox(height: 24),
                                    Row(
                                      children: [
                                        Container(
                                          width: 8,
                                          height: 8,
                                          decoration: BoxDecoration(
                                            color: _ollamaHealthy
                                                ? TailwindColors.tertiary
                                                : TailwindColors.error,
                                            shape: BoxShape.circle,
                                          ),
                                        ),
                                        const SizedBox(width: 8),
                                        Text(
                                          _ollamaHealthy
                                              ? 'CONNECTION STABLE'
                                              : 'CONNECTION ERROR',
                                          style: TextStyle(
                                            fontSize: 11,
                                            fontWeight: FontWeight.bold,
                                            color: _ollamaHealthy
                                                ? TailwindColors.tertiary
                                                : TailwindColors.error,
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

  Widget _buildTagAutocompleteField(
    String label,
    String? hint,
    TextEditingController controller,
  ) {
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
        RawAutocomplete<String>(
          textEditingController: controller,
          focusNode: FocusNode(),
          optionsBuilder: (TextEditingValue textEditingValue) {
            if (textEditingValue.text.isEmpty) {
              return const Iterable<String>.empty();
            }
            return _availableTags
                .map((t) => t.name)
                .where(
                  (name) => name.toLowerCase().contains(
                    textEditingValue.text.toLowerCase(),
                  ),
                );
          },
          fieldViewBuilder:
              (
                context,
                fieldTextEditingController,
                fieldFocusNode,
                onFieldSubmitted,
              ) {
                return TextField(
                  controller: fieldTextEditingController,
                  focusNode: fieldFocusNode,
                  onSubmitted: (String value) {
                    onFieldSubmitted();
                  },
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
                  ),
                );
              },
          optionsViewBuilder: (context, onSelected, options) {
            return Align(
              alignment: Alignment.topLeft,
              child: Material(
                elevation: 4,
                borderRadius: BorderRadius.circular(12),
                child: Container(
                  width: 300,
                  decoration: BoxDecoration(
                    color: TailwindColors.surfaceContainerHighest,
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: ListView.builder(
                    padding: const EdgeInsets.all(8),
                    shrinkWrap: true,
                    itemCount: options.length,
                    itemBuilder: (context, index) {
                      final option = options.elementAt(index);
                      return InkWell(
                        onTap: () => onSelected(option),
                        child: Padding(
                          padding: const EdgeInsets.all(12),
                          child: Text(
                            option,
                            style: const TextStyle(fontFamily: 'monospace'),
                          ),
                        ),
                      );
                    },
                  ),
                ),
              ),
            );
          },
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
